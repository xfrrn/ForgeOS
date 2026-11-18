package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRoleClaimRetainsRequestAcrossRestart(t *testing.T) {
	for _, role := range []string{"management", "curator", "test-agent", "reviewer"} {
		for _, mode := range []string{"unavailable", "lost_ack", "truncated", "legacy"} {
			t.Run(role+"/"+mode, func(t *testing.T) {
				t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
				var mu sync.Mutex
				requests := 0
				key := ""
				path := "/internal/" + role + "/claim?supports_process=1"
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					requests++
					if key == "" {
						key = r.Header.Get("X-ForgeOS-Claim-Key")
					}
					body, _ := io.ReadAll(r.Body)
					if len(key) != 32 || r.Header.Get("X-ForgeOS-Claim-Key") != key || r.Header.Get("X-ForgeOS-Node") != "claim-node" || r.URL.RequestURI() != path || string(body) != `{"original":true}` {
						t.Error("claim identity or frozen request changed")
					}
					if requests == 1 && mode != "legacy" {
						if mode == "unavailable" {
							w.WriteHeader(503)
							return
						}
						if mode == "truncated" {
							w.Header().Set("Content-Length", "100")
							w.WriteHeader(200)
							fmt.Fprint(w, `{"id":`)
							return
						}
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						conn.Close()
						return
					}
					if mode != "legacy" {
						w.Header().Set("X-ForgeOS-Claim-Key", key)
					}
					fmt.Fprint(w, `{"id":7,"lease":"12345678-1234-1234-1234-123456789abc"}`)
				}))
				defer server.Close()
				w := worker{api: server.URL, client: server.Client(), id: "claim-node"}
				var result struct {
					ID    int
					Lease string
				}
				status, err := w.claimRole(context.Background(), role, path, map[string]bool{"original": true}, &result)
				if mode != "legacy" {
					if status == 200 && err == nil {
						t.Fatal("incomplete claim accepted")
					}
					restarted := worker{api: server.URL, client: server.Client(), id: "claim-node"}
					status, err = restarted.claimRole(context.Background(), role, "/internal/"+role+"/claim?new_capability=1", struct{}{}, &result)
				}
				if err != nil || status != 200 || result.ID != 7 || len(result.Lease) != 36 {
					t.Fatal("restart did not recover", status, err)
				}
				db, _, err := w.openRoleJournal(context.Background(), role)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var count int
				if err = db.QueryRow("SELECT count(*) FROM role_claim_request").Scan(&count); err != nil || count != 0 {
					t.Fatal("confirmed claim remains pending", err)
				}
			})
		}
	}
}

func TestRoleClaimConcurrentDaemonsConsumeOnce(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var mu sync.Mutex
	var keys []string
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("X-ForgeOS-Claim-Key"))
		if len(keys) == 2 {
			close(ready)
		}
		mu.Unlock()
		<-ready
		w.Header().Set("X-ForgeOS-Claim-Key", r.Header.Get("X-ForgeOS-Claim-Key"))
		json.NewEncoder(w).Encode(map[string]any{"id": 7, "lease": "12345678-1234-1234-1234-123456789abc"})
	}))
	defer server.Close()
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := worker{api: server.URL, client: server.Client(), id: "same-node"}
			var out any
			status, err := w.claimRole(context.Background(), "curator", "/internal/curator/claim", struct{}{}, &out)
			if err != nil {
				t.Error(err)
			}
			statuses <- status
		}()
	}
	wg.Wait()
	close(statuses)
	success, empty := 0, 0
	for status := range statuses {
		if status == 200 {
			success++
		}
		if status == 204 {
			empty++
		}
	}
	if success != 1 || empty != 1 || len(keys) != 2 || keys[0] != keys[1] || strings.TrimSpace(keys[0]) == "" {
		t.Fatal("duplicate local assignment", success, empty)
	}
}
