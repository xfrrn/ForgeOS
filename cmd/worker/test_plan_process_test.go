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

	"forgeos/internal/acceptance"
	"forgeos/internal/store"
	"forgeos/internal/validation"
	"forgeos/schemas"
)

func TestCandidateRoleProcessExitsBeforeResult(t *testing.T) {
	for _, role := range []string{"test", "management"} {
		for _, mode := range []string{"current", "FAIL_PROCESS", "legacy"} {
			t.Run(role+"/"+mode, func(t *testing.T) {
				t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
				lib, e := schemas.GlobalPlaybooks()
				if e != nil {
					t.Fatal(e)
				}
				basis, e := acceptance.NewBasis(validation.Decision{}, lib, []string{"SIMULATED_TEST_PLAN_PROCESS " + mode})
				if e != nil {
					t.Fatal(e)
				}
				job := store.TestPlanJob{ID: 9, Lease: "12345678-1234-1234-1234-123456789abc", CaptureProcess: mode != "legacy", CaptureProfile: true, Source: acceptance.Source{Basis: basis, Library: lib}}
				if role == "management" {
					plan, e := basis.Bind("prior-test-agent", lib, acceptance.Draft{Cases: []acceptance.Case{}, Questions: []acceptance.Question{{Criterion: "requirement/1", Text: "Which observation is required?"}}})
					if e != nil {
						t.Fatal(e)
					}
					job.Plan = &plan
				}
				var mu sync.Mutex
				var reports []store.ManagementProcessReport
				results := 0
				endpoint := "/internal/test-agent"
				resultPath := endpoint + "/result"
				if role == "management" {
					endpoint = "/internal/management"
					resultPath = endpoint + "/test-plans/result"
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					switch r.URL.Path {
					case endpoint + "/process":
						var p store.ManagementProcessReport
						if json.NewDecoder(r.Body).Decode(&p) != nil || p.TestPlanID != job.ID || p.TestPlanRole != role || !p.Process.Valid() {
							t.Error("invalid process report")
						}
						reports = append(reports, p)
					case resultPath:
						var result store.TestPlanOutcome
						if json.NewDecoder(r.Body).Decode(&result) != nil {
							t.Error("invalid result")
						}
						if job.CaptureProcess && (len(reports) == 0 || reports[len(reports)-1].Process.Exited == nil) {
							t.Error("result before native exit")
						}
						if result.Failed != (mode == "FAIL_PROCESS") || (result.Profile != nil) == result.Failed {
							t.Error("failure or profile mismatch")
						}
						if !result.Failed && ((result.Plan != nil) != (role == "test") || (result.Coverage != nil) != (role == "management")) {
							t.Error("wrong outcome")
						}
						results++
					default:
						t.Error("wrong endpoint", r.URL.Path)
					}
					w.WriteHeader(204)
				}))
				defer server.Close()
				w := worker{api: server.URL, client: server.Client(), codex: os.Args[0], id: "candidate-node"}
				if e = w.finishTestJob(context.Background(), job, role == "management"); e != nil {
					t.Fatal(e)
				}
				mu.Lock()
				defer mu.Unlock()
				if results != 1 {
					t.Fatal("missing outcome")
				}
				if mode == "legacy" {
					if len(reports) != 0 {
						t.Fatal("legacy process")
					}
					return
				}
				last := reports[len(reports)-1].Process
				if last.Exited == nil || last.ExitCode == nil || (*last.ExitCode == 0) == (mode == "FAIL_PROCESS") || last.Output == nil || strings.Contains(last.Output.Text, "private-sample") {
					t.Fatal("wrong exit or public output")
				}
			})
		}
	}
}
