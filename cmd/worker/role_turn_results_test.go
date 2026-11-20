package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"forgeos/internal/store"
)

func TestCompletedRoleTurnRecoversBeforeFinalReceipt(t *testing.T) {
	for _, role := range []string{"management", managementChatRole, "curator", "test-agent", "reviewer"} {
		t.Run(role, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			target := store.ManagementProcessReport{Lease: "12345678-1234-1234-1234-123456789abc", TaskID: 5, Revision: 2}
			switch role {
			case "curator":
				target.TaskID, target.Revision, target.CurationID, target.CurationRole, target.Turn = 0, 0, 9, "curator", 1
			case "test-agent":
				target.TaskID, target.Revision, target.TestPlanID, target.TestPlanRole = 0, 0, 8, "test"
			case "reviewer":
				target.TaskID, target.Revision, target.ReviewerExecution = 0, 0, 7
			}
			var mu sync.Mutex
			recovering := false
			claims := 0
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if recovering {
					requests = append(requests, r.URL.Path)
				}
				if strings.HasSuffix(r.URL.Path, "/claim") {
					claims++
					if claims == 1 {
						json.NewEncoder(rw).Encode(map[string]any{"lease": target.Lease, "frozen": "first input"})
						return
					}
				}
				if strings.HasSuffix(r.URL.Path, "/process") && !recovering {
					var report store.ManagementProcessReport
					if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
						t.Error(err)
					}
					if report.Process.Exited != nil {
						rw.WriteHeader(503)
						return
					}
				}
				rw.WriteHeader(204)
			}))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client(), codex: os.Args[0], id: "completed-node", role: role}
			var job json.RawMessage
			if status, err := w.claimRole(context.Background(), role, "/internal/"+role+"/claim", struct{}{}, &job); err != nil || status != 200 {
				t.Fatal(status, err)
			}
			var result json.RawMessage
			session, err := w.roleProcessTurn(context.Background(), target, "codex", "1.2.3", &result, func(ctx context.Context, p processLog) (string, error) {
				return recordedJSONTurn(ctx, w.codex, reviewSchema, "SIMULATED_MANAGEMENT_PROCESS success", 12<<10, &result, p)
			})
			if err != nil {
				t.Fatal(err)
			}
			db, _, err := w.openRoleJournal(context.Background(), role)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var n int
			if err = db.QueryRow("SELECT count(*) FROM role_receipts").Scan(&n); err != nil || n != 0 {
				t.Fatal("final result already queued", err)
			}
			mu.Lock()
			recovering = true
			mu.Unlock()
			resumed := worker{api: w.api, client: w.client, id: w.id, codex: filepath.Join(t.TempDir(), "must-not-run"), role: role}
			if err = resumed.replayRoleReceipts(context.Background(), role); err != nil {
				t.Fatal(err)
			}
			var recoveredJob json.RawMessage
			if status, err := resumed.claimRole(context.Background(), role, "/internal/"+role+"/claim", struct{}{}, &recoveredJob); err != nil || status != 200 {
				t.Fatal(status, err)
			}
			if string(job) != string(recoveredJob) {
				t.Fatal("assignment changed")
			}
			var recovered json.RawMessage
			got, err := resumed.roleJSONTurn(context.Background(), true, target, reviewSchema, "must not execute", 12<<10, &recovered)
			if err != nil || got != session || string(result) != string(recovered) {
				t.Fatal("completed result not recovered", got, err)
			}
			if _, err = resumed.roleReceipt(context.Background(), "/internal/"+role+"/result", map[string]any{"lease": target.Lease, "result": recovered}); err != nil {
				t.Fatal(err)
			}
			if status, err := resumed.claimRole(context.Background(), role, "/internal/"+role+"/claim", struct{}{}, &job); err != nil || status != 204 {
				t.Fatal(status, err)
			}
			if err = db.QueryRow("SELECT count(*) FROM harness_processes").Scan(&n); err != nil || n != 1 {
				t.Fatal("model ran twice", n, err)
			}
			if err = db.QueryRow("SELECT count(*) FROM role_turn_results").Scan(&n); err != nil || n != 0 {
				t.Fatal("settled output retained", n, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if role == managementChatRole {
				if claims != 2 || len(requests) != 2 || requests[0] != managementChatResultPath || !strings.HasSuffix(requests[1], "/claim") {
					t.Fatal("chat recovery should only deliver the answer", claims, requests)
				}
				return
			}
			if claims != 2 || len(requests) != 3 || !strings.HasSuffix(requests[0], "/process") || !strings.HasSuffix(requests[1], "/result") || !strings.HasSuffix(requests[2], "/claim") {
				t.Fatal("wrong recovery order", claims, requests)
			}
		})
	}
}

func TestCompletedRoleTurnDoesNotHideUnfinishedRepair(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	lease := "12345678-1234-1234-1234-123456789abc"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"lease": lease})
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client(), id: "repair-node"}
	var job any
	if status, err := w.claimRole(context.Background(), "curator", "/internal/curator/claim", struct{}{}, &job); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	db, _, err := w.openRoleJournal(context.Background(), "curator")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := sha256.Sum256([]byte(lease))
	if err = w.markRoleStarted(db, key[:], 1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE role_turn_results SET session_id='first',result=? WHERE turn=1", []byte("{\"facts\":[]}")); err != nil {
		t.Fatal(err)
	}
	if err = w.markRoleStarted(db, key[:], 1); err == nil {
		t.Fatal("starting the same turn erased its completed result")
	}
	if err = w.markRoleStarted(db, key[:], 2); err != nil {
		t.Fatal(err)
	}
	resumed := worker{api: w.api, client: w.client, id: w.id}
	if status, err := resumed.claimRole(context.Background(), "curator", "/internal/curator/claim", struct{}{}, &job); err == nil || status != 409 {
		t.Fatal("unfinished second turn reran", status, err)
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM role_turn_results WHERE result IS NOT NULL").Scan(&n); err != nil || n != 1 {
		t.Fatal("first result lost", n, err)
	}
}

func TestCompletedRoleTurnAndExitRollbackTogether(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	target := store.ManagementProcessReport{Lease: "12345678-1234-1234-1234-123456789abc", TaskID: 5, Revision: 2}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/claim") {
			json.NewEncoder(w).Encode(map[string]any{"lease": target.Lease})
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client(), id: "rollback-node", codex: os.Args[0]}
	var job any
	if status, err := w.claimRole(context.Background(), "management", "/internal/management/claim", struct{}{}, &job); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	db, _, err := w.openRoleJournal(context.Background(), "management")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TRIGGER reject_completed BEFORE UPDATE ON role_turn_results WHEN NEW.result IS NOT NULL BEGIN SELECT RAISE(ABORT,'injected completion failure'); END"); err != nil {
		t.Fatal(err)
	}
	var result json.RawMessage
	_, err = w.roleProcessTurn(context.Background(), target, "codex", "1.2.3", &result, func(ctx context.Context, p processLog) (string, error) {
		return recordedJSONTurn(ctx, w.codex, reviewSchema, "SIMULATED_MANAGEMENT_PROCESS success", 12<<10, &result, p)
	})
	if err == nil || !strings.Contains(err.Error(), "injected completion failure") {
		t.Fatal("injected failure hidden", err)
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM role_turn_results WHERE result IS NOT NULL").Scan(&n); err != nil || n != 0 {
		t.Fatal("partial completion", n, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM role_process_receipts").Scan(&n); err != nil || n != 0 {
		t.Fatal("exit transaction not rolled back", n, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM harness_processes WHERE exit_code=0").Scan(&n); err != nil || n != 1 {
		t.Fatal("actual exit lost", n, err)
	}
	resumed := worker{api: w.api, client: w.client, id: w.id}
	if status, err := resumed.claimRole(context.Background(), "management", "/internal/management/claim", struct{}{}, &job); err == nil || status != 409 {
		t.Fatal("unpersisted completion restarted", status, err)
	}
}
