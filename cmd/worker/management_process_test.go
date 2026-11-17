package main

import (
	"context"
	"encoding/json"
	"forgeos/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestManagementProcessReportsNativeOutputAndExit(t *testing.T) {
	for _, mode := range []string{"success", "FAIL_PROCESS", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			var mu sync.Mutex
			var reports []store.ManagementProcessReport
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var p store.ManagementProcessReport
				if r.URL.Path != "/internal/management/process" || r.Header.Get("Authorization") != "Bearer private-manager" || json.NewDecoder(r.Body).Decode(&p) != nil {
					t.Error("invalid report")
				}
				mu.Lock()
				reports = append(reports, p)
				mu.Unlock()
				w.WriteHeader(204)
			}))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client(), codex: os.Args[0], token: "private-manager", id: "review-node"}
			job := store.ManagementJob{CaptureProcess: mode != "legacy", TaskID: 5, Revision: 2, Lease: "12345678-1234-1234-1234-123456789abc", Spec: store.IssueSpec{Issue: store.Issue{Title: "SIMULATED_MANAGEMENT_PROCESS " + mode}}}
			_, err := w.reviewManagement(context.Background(), job)
			if (mode == "FAIL_PROCESS") != (err != nil) {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if mode == "legacy" {
				if len(reports) != 0 {
					t.Fatal("legacy reported")
				}
				return
			}
			if len(reports) < 2 {
				t.Fatal("missing live and final reports")
			}
			live := false
			for _, r := range reports {
				p := r.Process
				if !p.Valid() || r.TaskID != 5 || r.Revision != 2 || r.Node != "review-node" || r.Lease != job.Lease {
					t.Fatal(r)
				}
				if p.Output != nil {
					if strings.Contains(p.Output.Text, "private-") {
						t.Fatal("secret in output")
					}
					live = live || p.Exited == nil
				}
			}
			final := reports[len(reports)-1].Process
			if !live || final.Exited == nil || final.Output == nil || final.Session != "planner-test" || (*final.ExitCode == 0) != (mode == "success") {
				t.Fatal(final, live)
			}
		})
	}
}
