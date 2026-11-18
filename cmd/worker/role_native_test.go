package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

func TestRoleNativeSessionResumesFrozenTurn(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "resume"
		if missing {
			name = "handoff"
		}
		t.Run(name, func(t *testing.T) { testRoleNativeRecovery(t, missing) })
	}
}
func testRoleNativeRecovery(t *testing.T, missing bool) {
	prompt := "SIMULATED_MANAGEMENT_PROCESS SIMULATED_NATIVE_ROLE"
	if missing {
		prompt += "_MISSING"
	}
	prompt += " success"

	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	lease := "12345678-1234-1234-1234-123456789abc"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/claim") {
			json.NewEncoder(w).Encode(map[string]any{"lease": lease, "recover_native": true})
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client(), id: "native-node", codex: os.Args[0]}
	var job any
	if status, err := w.claimRole(context.Background(), "management", "/internal/management/claim", struct{}{}, &job); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	target := store.ManagementProcessReport{TaskID: 1, Revision: 1, Lease: lease}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		var result json.RawMessage
		_, err := w.roleJSONTurn(ctx, true, target, reviewSchema, prompt, 12<<10, &result)
		done <- err
	}()
	db, _, err := w.openRoleJournal(context.Background(), "management")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	deadline := time.Now().Add(10 * time.Second)
	var sid string
	for time.Now().Before(deadline) {
		_ = db.QueryRow("SELECT COALESCE(session_id,'') FROM harness_processes ORDER BY instance DESC LIMIT 1").Scan(&sid)
		if sid != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if sid == "" {
		cancel()
		<-done
		t.Fatal("missing original session")
	}
	// An alive process cannot be taken over even by another Daemon with the same identity.
	blocked := worker{api: w.api, client: w.client, id: w.id, codex: w.codex}
	if status, err := blocked.claimRole(context.Background(), "management", "/internal/management/claim", struct{}{}, &job); err == nil || status != 409 {
		t.Fatal("active native process accepted", status, err)
	}
	cancel()
	if err = <-done; err == nil {
		t.Fatal("interrupted turn succeeded")
	}
	resumed := worker{api: w.api, client: w.client, id: w.id, codex: w.codex}

	var version string
	if err = db.QueryRow("SELECT version FROM role_native_turns").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE role_native_turns SET version='unsupported-test-version'"); err != nil {
		t.Fatal(err)
	}
	if status, err := resumed.claimRole(context.Background(), "management", "/internal/management/claim", struct{}{}, &job); err == nil || status != 409 || !strings.Contains(err.Error(), "version changed") {
		t.Fatal("changed Harness version accepted", status, err)
	}
	if _, err = db.Exec("UPDATE role_native_turns SET version=?", version); err != nil {
		t.Fatal(err)
	}
	if status, err := resumed.claimRole(context.Background(), "management", "/internal/management/claim", struct{}{}, &job); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	var result json.RawMessage
	got, err := resumed.roleJSONTurn(context.Background(), true, target, "new schema must not replace old", "changed input must not replace original", 12<<10, &result)
	if err != nil || (got == sid) == missing || !strings.Contains(string(result), "Handbook") {
		t.Fatal("native recovery", got, string(result), err)
	}
	wantProcesses, wantSessions := 2, 1
	if missing {
		wantProcesses, wantSessions = 3, 2
	}
	var processes, sessions int
	if err = db.QueryRow("SELECT count(*),count(DISTINCT session_id) FROM harness_processes").Scan(&processes, &sessions); err != nil || processes != wantProcesses || sessions != wantSessions {
		t.Fatal("history", processes, sessions, err)
	}
	var savedPrompt, contract string
	if err = db.QueryRow("SELECT prompt,contract FROM role_native_turns").Scan(&savedPrompt, &contract); err != nil || savedPrompt != prompt || contract != reviewSchema {
		t.Fatal("changed frozen input", err)
	}
}

func TestRoleNativeInterruptedChildRetriesSameSession(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	lease := "12345678-1234-1234-1234-123456789abc"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/claim") {
			json.NewEncoder(w).Encode(map[string]any{"lease": lease, "recover_native": true})
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client(), id: "native-node", codex: os.Args[0]}
	var job any
	if status, err := w.claimRole(context.Background(), "management", "/internal/management/claim", struct{}{}, &job); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	target := store.ManagementProcessReport{TaskID: 1, Revision: 1, Lease: lease}
	var result json.RawMessage
	sid, err := w.roleJSONTurn(context.Background(), true, target, reviewSchema, "SIMULATED_MANAGEMENT_PROCESS SIMULATED_NATIVE_ROLE_INTERRUPT", 12<<10, &result)
	if err != nil || !nativeSessionID.MatchString(sid) || !strings.Contains(string(result), "Handbook") {
		t.Fatal("interrupted native turn not recovered", sid, err)
	}
	db, _, err := w.openRoleJournal(context.Background(), "management")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var processes, sessions int
	if err = db.QueryRow("SELECT count(*),count(DISTINCT session_id) FROM harness_processes").Scan(&processes, &sessions); err != nil || processes != 2 || sessions != 1 {
		t.Fatal("retry history", processes, sessions, err)
	}
}
