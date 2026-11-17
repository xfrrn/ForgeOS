package main

import (
	"context"
	"encoding/json"
	"forgeos/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestQuestionAdviceRecordsIndependentProcess(t *testing.T) {
	for _, mode := range []string{"success", "FAIL_PROCESS", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			var mu sync.Mutex
			var processes []store.HarnessProcess
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				var p store.ManagementProcessReport
				if r.URL.Path != "/internal/management/process" || json.NewDecoder(r.Body).Decode(&p) != nil || p.QuestionID != 7 || p.TaskID != 0 || p.AnalysisTask != 0 || p.ShareID != 0 || !p.Process.Valid() {
					t.Error("invalid advice process")
				}
				processes = append(processes, p.Process)
				w.WriteHeader(204)
			}))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client(), codex: os.Args[0], token: "private-manager", id: "advice-node"}
			job := store.ManagementJob{CaptureProcess: mode != "legacy", Lease: "12345678-1234-1234-1234-123456789abc", MinimumLevel: "D2", Question: &store.QuestionAdviceJob{ID: 7, Question: "SIMULATED_QUESTION_CONTEXT SIMULATED_ADVICE_PROCESS " + mode, Sources: json.RawMessage(`[{"id":"question","text":"Owner chooses heading."}]`)}}
			a, e := w.adviseOwner(context.Background(), job)
			if (e != nil) != (mode == "FAIL_PROCESS") {
				t.Fatal(e)
			}
			if e == nil && (a.Session != "planner-test" || a.Quote != "Owner chooses heading.") {
				t.Fatal(a)
			}
			mu.Lock()
			defer mu.Unlock()
			if mode == "legacy" {
				if len(processes) != 0 {
					t.Fatal("legacy process")
				}
				return
			}
			if len(processes) < 2 {
				t.Fatal("missing live observation")
			}
			p := processes[len(processes)-1]
			if p.Exited == nil || p.ExitCode == nil || (*p.ExitCode == 0) != (mode != "FAIL_PROCESS") || p.Output == nil || p.Session != "planner-test" {
				t.Fatal(p)
			}
		})
	}
}
