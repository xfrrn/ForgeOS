package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRoleReceiptsReplayAfterRestart(t *testing.T) {
	paths := []string{"/internal/management/result", "/internal/management/questions/result", "/internal/management/feedback/result", "/internal/management/issue-classifications/result", "/internal/management/repository-analysis/result", "/internal/management/test-plans/result", "/internal/management/curation/result", "/internal/curator/result", "/internal/test-agent/result", "/internal/reviewer/result"}
	for _, path := range paths {
		for _, mode := range []string{"unavailable", "lost_ack", "conflict", "unauthorized"} {
			t.Run(path+"/"+mode, func(t *testing.T) {
				t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
				ctx, stop := context.WithCancel(context.Background())
				defer stop()
				body := map[string]any{"lease": "12345678-1234-1234-1234-123456789abc", "id": 7, "session": "native-session"}
				raw, _ := json.Marshal(body)
				var mu sync.Mutex
				requests := 0
				replay := false
				server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					if r.URL.Path != path {
						t.Error("incorrect replay target")
					}
					var got map[string]any
					if json.NewDecoder(r.Body).Decode(&got) != nil {
						t.Error("invalid payload")
					}
					b, _ := json.Marshal(got)
					if string(b) != string(raw) {
						t.Error("changed result")
					}
					requests++
					if !replay {
						if mode == "unauthorized" {
							rw.WriteHeader(401)
							return
						}
						if mode == "lost_ack" {
							conn, _, err := rw.(http.Hijacker).Hijack()
							if err != nil {
								t.Error(err)
								return
							}
							conn.Close()
							stop()
							return
						}
						rw.WriteHeader(503)
						stop()
						return
					}
					if mode == "conflict" {
						rw.WriteHeader(409)
					} else {
						rw.WriteHeader(204)
					}
				}))
				defer server.Close()
				w := worker{api: server.URL, client: server.Client(), id: "role-node"}
				if _, err := w.roleReceipt(ctx, path, body); err == nil {
					t.Fatal("unconfirmed result returned success")
				}
				role := roleForReceipt(path)
				db, _, err := w.openRoleJournal(context.Background(), role)
				if err != nil {
					t.Fatal(err)
				}
				var pending []byte
				if err = db.QueryRow("SELECT report FROM role_receipts").Scan(&pending); err != nil || string(pending) != string(raw) {
					t.Fatal("lost buffered payload", err)
				}
				db.Close()
				other := w
				other.id = "another-node"
				if err = other.replayRoleReceipts(context.Background(), role); err != nil {
					t.Fatal(err)
				}
				otherRole := "curator"
				if role == otherRole {
					otherRole = "reviewer"
				}
				if err = w.replayRoleReceipts(context.Background(), otherRole); err != nil {
					t.Fatal(err)
				}
				mu.Lock()
				if requests != 1 {
					t.Error("cross-node or cross-role replay")
				}
				replay = true
				mu.Unlock()
				restarted := worker{api: server.URL, client: server.Client(), id: "role-node"}
				if err = restarted.replayRoleReceipts(context.Background(), role); err != nil {
					t.Fatal(err)
				}
				if err = restarted.replayRoleReceipts(context.Background(), role); err != nil {
					t.Fatal(err)
				}
				status, err := restarted.roleReceipt(context.Background(), path, body)
				want := 204
				if mode == "conflict" {
					want = 409
				}
				if err != nil || status != want {
					t.Fatal("settled replay", status, err)
				}
				mu.Lock()
				if requests != 2 {
					t.Error("settled result resent", requests)
				}
				mu.Unlock()
				changed := map[string]any{"lease": body["lease"], "id": 7, "session": "invented-new-session"}
				if _, err = restarted.roleReceipt(context.Background(), path, changed); err == nil {
					t.Fatal("replaced lease result")
				}
				db, _, err = restarted.openRoleJournal(context.Background(), role)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var report []byte
				var savedStatus int
				var conflict string
				if err = db.QueryRow("SELECT report,status,coalesce(conflicting_result,'') FROM role_receipts").Scan(&report, &savedStatus, &conflict); err != nil {
					t.Fatal(err)
				}
				if report != nil || savedStatus != want || strings.Contains(conflict, body["lease"].(string)) || (conflict != "") != (mode == "conflict") {
					t.Fatal("settlement retained lease or lost conflict")
				}
			})
		}
	}
}

func TestRoleReceiptBuffersCancelledCompletionAndRejectsOtherEndpoints(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w := worker{api: "http://127.0.0.1:1", id: "role"}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	body := map[string]any{"lease": "12345678-1234-1234-1234-123456789abc", "id": 9}
	if _, err := w.roleReceipt(ctx, "/internal/curator/result", body); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	db, _, err := w.openRoleJournal(context.Background(), "curator")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err = db.QueryRow("SELECT count(*) FROM role_receipts WHERE report IS NOT NULL").Scan(&n); err != nil || n != 1 {
		t.Fatal("cancelled completion lost", err)
	}
	for _, path := range []string{"/internal/curator/claim", "/internal/management/process", "/tasks/1/acceptance", "https://example.invalid/"} {
		if _, err = w.roleReceipt(ctx, path, body); err == nil {
			t.Fatal("arbitrary replay target")
		}
	}
	if _, err = w.roleReceipt(ctx, "/internal/curator/result", map[string]any{"lease": "bad"}); err == nil {
		t.Fatal("invalid lease")
	}
}

func TestRoleJournalUpgradeKeepsPriorNativeProcesses(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w := worker{api: "http://127.0.0.1:1", id: "role"}
	db, _, err := w.openRoleJournal(context.Background(), "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`DROP TABLE role_receipts;
 INSERT INTO harness_processes(lease_hash,harness,version,pid,session_id,exited_at,exit_code)
 VALUES(zeroblob(32),'claude','2.1.310',918,'prior-session',CURRENT_TIMESTAMP,0)`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	// Startup adds the receipt queue to the pre-existing role journal.
	if err = w.replayRoleReceipts(context.Background(), "reviewer"); err != nil {
		t.Fatal(err)
	}
	db, _, err = w.openRoleJournal(context.Background(), "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var pid, exit int
	var session string
	if err = db.QueryRow("SELECT pid,session_id,exit_code FROM harness_processes").Scan(&pid, &session, &exit); err != nil || pid != 918 || session != "prior-session" || exit != 0 {
		t.Fatal("changed prior process", err)
	}
}
