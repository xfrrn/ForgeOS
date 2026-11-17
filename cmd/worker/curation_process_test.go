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

func TestCurationProcessIncludesRepairAndSeparateReview(t *testing.T) {
	for _, mode := range []string{"repair", "review", "FAIL_PROCESS", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			var mu sync.Mutex
			var observed []store.ManagementProcessReport
			var results int
			role := "curator"
			endpoint := "/internal/curator/result"
			if mode == "review" {
				role = "management"
				endpoint = "/internal/management/curation/result"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.URL.Path == "/internal/"+role+"/process" {
					var p store.ManagementProcessReport
					if json.NewDecoder(r.Body).Decode(&p) != nil || p.CurationID != 7 || p.CurationRole != role || p.Turn < 1 || p.Turn > 2 || !p.Process.Valid() {
						t.Error("invalid curation process")
					}
					observed = append(observed, p)
				} else if r.URL.Path == endpoint {
					if mode != "legacy" && (len(observed) == 0 || observed[len(observed)-1].Process.Exited == nil) {
						t.Error("result before exit")
					}
					results++
				} else {
					t.Error("wrong role endpoint", r.URL.Path)
				}
				w.WriteHeader(204)
			}))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client(), codex: os.Args[0], token: "private-role", id: "knowledge-node"}
			source := "SIMULATED_CURATOR_PROCESS SIMULATED_QUOTE_REPAIR saved 918 then resumed at 918 " + mode
			if mode == "review" {
				source = "SIMULATED_CURATOR_PROCESS SIMULATED_CURATOR_REVIEW resumed at 918"
			}
			material, _ := json.Marshal(map[string]string{"content": source})
			job := store.CurationJob{ID: 7, Lease: "12345678-1234-1234-1234-123456789abc", CaptureProcess: mode != "legacy", Material: material}
			if mode == "review" {
				job.Extraction = &store.Extraction{Session: "prior-extraction", Facts: []store.KnowledgeFact{}}
			}
			if e := w.curate(context.Background(), job, mode == "review"); e != nil {
				t.Fatal(e)
			}
			mu.Lock()
			defer mu.Unlock()
			if (results == 1) != (mode != "FAIL_PROCESS") {
				t.Fatal("result count", results)
			}
			if mode == "legacy" {
				if len(observed) != 0 {
					t.Fatal("legacy process")
				}
				return
			}
			ended := map[int]store.HarnessProcess{}
			for _, p := range observed {
				if p.Process.Exited != nil {
					ended[p.Turn] = p.Process
				}
			}
			if mode == "repair" {
				if len(ended) != 2 || ended[1].Session != "planner-test" || ended[2].Session != "curator-repaired" || ended[2].Instance <= ended[1].Instance {
					t.Fatal("repair history", ended)
				}
			} else if len(ended) != 1 {
				t.Fatal("role history", ended)
			}
			for _, p := range ended {
				if p.ExitCode == nil || (*p.ExitCode == 0) != (mode != "FAIL_PROCESS") || p.Output == nil {
					t.Fatal("native exit", p)
				}
			}
		})
	}
}
