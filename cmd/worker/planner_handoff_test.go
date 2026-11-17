package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type unavailablePlanner struct {
	err   error
	calls int
}

func (p *unavailablePlanner) identity() (string, string) { return "codex", "1.2.3" }

func (p *unavailablePlanner) runTurn(context.Context, processLog, string, string, string) (string, error) {
	p.calls++
	return "", p.err
}

func TestPlannerHandoffRequiresConfirmedLossAndCleanLease(t *testing.T) {
	const original = "e0749d55-e617-45ee-a6ef-8bd7fadb1c31"
	for _, tc := range []struct {
		name    string
		cause   error
		enabled bool
		renew   int
		dirty   bool
	}{
		{"ordinary failure", errTurnFailed, true, 0, false},
		{"older controller", errSessionMissing, false, 0, false},
		{"expired lease", errSessionMissing, true, http.StatusConflict, false},
		{"modified checkout", errSessionMissing, true, http.StatusNoContent, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, j, base := fixture(t)
			ctx := context.Background()
			dir, err := checkedWorktree(ctx, j.RepoPath, j.Workspace, base)
			if err != nil {
				t.Fatal(err)
			}
			if tc.dirty {
				if err := os.WriteFile(filepath.Join(j.Workspace, "external-edit"), []byte("Owner edit"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if tc.renew == 0 || r.URL.Path != "/internal/executions/12/renew" {
					t.Error("unexpected lease check", r.URL.Path)
					rw.WriteHeader(http.StatusBadRequest)
					return
				}
				rw.WriteHeader(tc.renew)
			}))
			defer server.Close()
			w.api, w.client = server.URL, server.Client()
			j.PlannerHandoff = tc.enabled
			adapter := &unavailablePlanner{err: tc.cause}
			session, err := w.runJobTurn(ctx, adapter, j, "Plan this issue", original, base, dir, nil, filepath.Join(t.TempDir(), "plan.json"))
			if err == nil || adapter.calls != 1 || session != original || (tc.renew == 0 && !errors.Is(err, tc.cause)) {
				t.Fatal("unexpected handoff attempt", adapter.calls, session, err)
			}
		})
	}
}
