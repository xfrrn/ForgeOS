package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
)

func TestRoleAssignmentRestartsBeforeNativeTurn(t *testing.T) {
	for _, role := range []string{"management", "curator", "test-agent", "reviewer"} {
		t.Run(role, func(t *testing.T) {
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			lease := "12345678-1234-1234-1234-123456789abc"
			var mu sync.Mutex
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				calls++
				json.NewEncoder(w).Encode(map[string]any{"id": 7, "lease": lease, "frozen": "original input"})
			}))
			defer server.Close()
			first := worker{api: server.URL, client: server.Client(), id: "assigned-node"}
			var original, recovered map[string]any
			path := "/internal/" + role + "/claim"
			if status, err := first.claimRole(context.Background(), role, path, struct{}{}, &original); err != nil || status != 200 {
				t.Fatal(status, err)
			}
			second := worker{api: server.URL, client: server.Client(), id: "assigned-node"}
			if status, err := second.claimRole(context.Background(), role, path+"?new=1", map[string]bool{"changed": true}, &recovered); err != nil || status != 200 {
				t.Fatal(status, err)
			}
			if original["lease"] != recovered["lease"] || recovered["frozen"] != "original input" {
				t.Fatal("changed assignment")
			}
			mu.Lock()
			n := calls
			mu.Unlock()
			if n != 1 {
				t.Fatal("allocated again", n)
			}
			if err := second.roleStartHook(role, lease)(); err != nil {
				t.Fatal(err)
			}
			third := worker{api: server.URL, client: server.Client(), id: "assigned-node"}
			if status, err := third.claimRole(context.Background(), role, path, struct{}{}, &recovered); err == nil || status != 409 {
				t.Fatal("restarted an already started turn", status, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if calls != 1 {
				t.Fatal("started assignment reached claim API")
			}
		})
	}
}

func TestRoleAssignmentSettledResultDoesNotRunAgain(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	lease := "12345678-1234-1234-1234-123456789abc"
	claims := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/curator/claim" {
			claims++
			if claims == 1 {
				json.NewEncoder(w).Encode(map[string]any{"id": 7, "lease": lease})
				return
			}
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client(), id: "assigned-node"}
	var out any
	if status, err := w.claimRole(context.Background(), "curator", "/internal/curator/claim", struct{}{}, &out); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	if err := w.roleStartHook("curator", lease)(); err != nil {
		t.Fatal(err)
	}
	if status, err := w.roleReceipt(context.Background(), "/internal/curator/result", map[string]any{"id": 7, "lease": lease}); err != nil || status != 204 {
		t.Fatal(status, err)
	}
	restarted := worker{api: server.URL, client: server.Client(), id: "assigned-node"}
	if status, err := restarted.claimRole(context.Background(), "curator", "/internal/curator/claim", struct{}{}, &out); err != nil || status != 204 {
		t.Fatal(status, err)
	}
	db, _, err := restarted.openRoleJournal(context.Background(), "curator")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err = db.QueryRow("SELECT count(*) FROM role_assignment").Scan(&n); err != nil || n != 0 {
		t.Fatal("settled assignment retained", err)
	}
}

func TestRoleDaemonLock(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	first := worker{api: "http://127.0.0.1:1", id: "locked-node"}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- first.runIndependentRole(context.Background(), "curator", func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	second := worker{api: first.api, id: first.id}
	called := false
	if err := second.runIndependentRole(context.Background(), "curator", func(context.Context) error { called = true; return nil }); err == nil || called {
		t.Error("duplicate role daemon entered")
	}
	if err := second.runIndependentRole(context.Background(), "reviewer", func(context.Context) error { return nil }); err != nil {
		t.Error("cross-role lock", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := second.runIndependentRole(context.Background(), "curator", func(context.Context) error { return nil }); err != nil {
		t.Fatal("lock survived owner exit", err)
	}
}
