package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func TestAnalysisRecordsActualExitBeforeItsResult(t *testing.T) {
	for _, mode := range []string{"success", "FAIL_PROCESS", "BAD_QUOTE", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			var mu sync.Mutex
			var processes []store.HarnessProcess
			var result store.AnalysisResult
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/internal/management/process":
					var report store.ManagementProcessReport
					if json.NewDecoder(r.Body).Decode(&report) != nil || report.AnalysisTask != 7 || report.TaskID != 0 || report.ShareID != 0 || !report.Process.Valid() {
						t.Error("bad analysis process")
					}
					if report.Process.Output != nil && strings.Contains(report.Process.Output.Text, "private-") {
						t.Error("unsafe public output")
					}
					processes = append(processes, report.Process)
				case "/internal/management/repository-analysis/result":
					if json.NewDecoder(r.Body).Decode(&result) != nil {
						t.Error("bad result")
					}
					if mode != "legacy" && (len(processes) == 0 || processes[len(processes)-1].Exited == nil) {
						t.Error("result preceded confirmed exit")
					}
				default:
					t.Error("unexpected endpoint")
				}
				w.WriteHeader(204)
			}))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client(), codex: os.Args[0], token: "private-manager", id: "analysis-node"}
			job := store.AnalysisJob{Task: 7, Lease: "12345678-1234-1234-1234-123456789abc", CaptureProcess: mode != "legacy", Input: localrepo.AnalysisInput{Sources: []localrepo.AnalysisSource{{Path: "README.md", Text: "SIMULATED_ANALYSIS_PROCESS " + mode + " go run ./cmd/app"}}}}
			if e := w.analyzeRepository(context.Background(), job); e != nil {
				t.Fatal(e)
			}
			mu.Lock()
			defer mu.Unlock()
			if mode == "FAIL_PROCESS" && result.Failure != "agent_run_failed" || mode == "BAD_QUOTE" && result.Failure != "unverified_evidence" || (mode == "success" || mode == "legacy") && (result.Analysis == nil || result.Analysis.Session != "planner-test") {
				t.Fatal("incorrect result", result)
			}
			if mode == "legacy" {
				if len(processes) != 0 {
					t.Fatal("legacy sent process")
				}
				return
			}
			if len(processes) < 2 {
				t.Fatal("missing live reports")
			}
			final := processes[len(processes)-1]
			if final.Session != "planner-test" || final.Output == nil || (*final.ExitCode == 0) != (mode != "FAIL_PROCESS") {
				t.Fatal(final)
			}
		})
	}
}
