package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"forgeos/internal/store"
)

func TestRoleExitDeliveryFailurePreservesCompletedResult(t *testing.T) {
	for _, role := range []string{"management", "curator", "test-agent", "reviewer"} {
		t.Run(role, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			var mu sync.Mutex
			recovering := false
			var sequence []string
			var exits []store.ManagementProcessReport
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if strings.HasSuffix(r.URL.Path, "/process") {
					var p store.ManagementProcessReport
					if json.NewDecoder(r.Body).Decode(&p) != nil {
						t.Error("invalid process")
					}
					if p.Process.Exited != nil {
						exits = append(exits, p)
						if !recovering {
							rw.WriteHeader(503)
							return
						}
					}
				}
				if recovering {
					sequence = append(sequence, r.URL.Path)
				}
				rw.WriteHeader(204)
			}))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client(), codex: os.Args[0], id: "exit-node"}
			target := store.ManagementProcessReport{Lease: "12345678-1234-1234-1234-123456789abc", TaskID: 5, Revision: 2}
			switch role {
			case "curator":
				target.TaskID, target.Revision, target.CurationID, target.CurationRole, target.Turn = 0, 0, 9, "curator", 1
			case "test-agent":
				target.TaskID, target.Revision, target.TestPlanID, target.TestPlanRole = 0, 0, 8, "test"
			case "reviewer":
				target.TaskID, target.Revision, target.ReviewerExecution = 0, 0, 7
			}
			var result json.RawMessage
			// This real test subprocess supplies a structured result; native role identity is covered separately.
			session, err := w.roleProcessTurn(context.Background(), target, "codex", "1.2.3", &result, func(ctx context.Context, p processLog) (string, error) {
				return recordedJSONTurn(ctx, w.codex, reviewSchema, "SIMULATED_MANAGEMENT_PROCESS success", 12<<10, &result, p)
			})
			if err != nil || session != "planner-test" || !strings.Contains(string(result), "Handbook") {
				t.Fatal("delivery failure discarded model success", session, err)
			}
			path := "/internal/" + role + "/result"
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err = w.roleReceipt(ctx, path, map[string]any{"lease": target.Lease, "result": result}); err == nil {
				t.Fatal("cancelled result confirmed")
			}
			mu.Lock()
			recovering = true
			mu.Unlock()
			restarted := worker{api: server.URL, client: server.Client(), id: "exit-node"}
			if err = restarted.replayRoleReceipts(context.Background(), role); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(sequence) != 2 || sequence[0] != "/internal/"+role+"/process" || sequence[1] != path || len(exits) < 2 {
				t.Fatal("exit must precede result", sequence)
			}
			first, _ := json.Marshal(exits[0])
			last, _ := json.Marshal(exits[len(exits)-1])
			if string(first) != string(last) {
				t.Fatal("exit changed on replay")
			}
			db, _, err := restarted.openRoleJournal(context.Background(), role)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var n int
			if err = db.QueryRow("SELECT count(*) FROM role_process_receipts WHERE status=204 AND report IS NULL").Scan(&n); err != nil || n != 1 {
				t.Fatal("exit receipt retained private lease", err)
			}
		})
	}
}

func TestRoleExitReplayAuthorizationConflictAndOrder(t *testing.T) {
	for _, mode := range []string{"lost_ack", "unauthorized", "expired"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var mu sync.Mutex
			recovering := false
			var order []int64
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				var report store.ManagementProcessReport
				if r.URL.Path != "/internal/curator/process" || json.NewDecoder(r.Body).Decode(&report) != nil {
					t.Error("invalid process route")
				}
				order = append(order, report.Process.Instance)
				if !recovering && mode == "unauthorized" {
					rw.WriteHeader(401)
					return
				}
				if !recovering && mode == "lost_ack" {
					conn, _, err := rw.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					conn.Close()
					cancel()
					return
				}
				if mode == "expired" {
					rw.WriteHeader(409)
				} else {
					rw.WriteHeader(204)
				}
			}))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client(), id: "exit-node"}
			db, _, err := w.openRoleJournal(context.Background(), "curator")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			start, end, code := time.Now().Add(-time.Second), time.Now(), 0
			for n := int64(1); n <= 2; n++ {
				report := store.ManagementProcessReport{Lease: "12345678-1234-1234-1234-123456789abc", Node: w.id, CurationID: 9, CurationRole: "curator", Turn: int(n), Process: store.HarnessProcess{Instance: n, PID: 900 + int(n), Harness: "codex", Version: "1.2.3", Session: "session", Started: start, Exited: &end, ExitCode: &code}}
				if err = bufferRoleProcess(db, report); err != nil {
					t.Fatal(err)
				}
			}
			err = w.replayRoleProcesses(ctx, db, "curator")
			if (mode != "expired") != (err != nil) {
				t.Fatal("unexpected exit replay outcome", err)
			}
			var pending int
			if err = db.QueryRow("SELECT count(*) FROM role_process_receipts WHERE report IS NOT NULL").Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if (pending == 2) != (mode != "expired") {
				t.Fatal("lost unconfirmed exits", pending)
			}
			mu.Lock()
			recovering = true
			mu.Unlock()
			if err = w.replayRoleProcesses(context.Background(), db, "curator"); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			want := "[1 1 2]"
			if mode == "expired" {
				want = "[1 2]"
			}
			if fmt.Sprint(order) != want {
				t.Fatal("reordered turn exits", order)
			}
			var conflict string
			if err = db.QueryRow("SELECT coalesce(conflicting_report,'') FROM role_process_receipts WHERE instance=1").Scan(&conflict); err != nil || strings.Contains(conflict, "12345678-1234") || (conflict != "") != (mode == "expired") {
				t.Fatal("incorrect private conflict record", err)
			}
		})
	}
}
