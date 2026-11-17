package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"forgeos/internal/store"
)

func TestFeedbackClassificationRecordsExitBeforeResult(t *testing.T) {
	for _, mode := range []string{"success", "FAIL_PROCESS", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			var mu sync.Mutex
			var processes []store.HarnessProcess
			var results int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/internal/management/process":
					var p store.ManagementProcessReport
					if json.NewDecoder(r.Body).Decode(&p) != nil || p.FeedbackID != 7 || p.TaskID != 0 || p.AnalysisTask != 0 || p.ShareID != 0 || p.QuestionID != 0 || !p.Process.Valid() {
						t.Error("invalid feedback process")
					}
					processes = append(processes, p.Process)
				case "/internal/management/feedback/result":
					var result struct{ Classification store.FeedbackClassification }
					if json.NewDecoder(r.Body).Decode(&result) != nil || result.Classification.Session != "planner-test" {
						t.Error("invalid classification")
					}
					if mode != "legacy" && (len(processes) == 0 || processes[len(processes)-1].Exited == nil) {
						t.Error("classification preceded exit")
					}
					results++
				default:
					t.Error("unexpected endpoint")
				}
				w.WriteHeader(204)
			}))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client(), codex: os.Args[0], token: "private-manager", id: "feedback-node"}
			source, _ := json.Marshal(map[string]any{"feedback": map[string]string{"note": "SIMULATED_FEEDBACK_CONTEXT SIMULATED_FEEDBACK_PROCESS " + mode}})
			job := store.FeedbackJob{CaptureProcess: mode != "legacy", Lease: "12345678-1234-1234-1234-123456789abc", ID: 7, Source: source}
			if e := w.classifyFeedback(context.Background(), job); e != nil {
				t.Fatal(e)
			}
			mu.Lock()
			defer mu.Unlock()
			if (results == 1) != (mode != "FAIL_PROCESS") {
				t.Fatal("incorrect classification count", results)
			}
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
