package main

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type searchRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *searchRecorder) SetWriteDeadline(until time.Time) error {
	w.deadline = until
	return nil
}

func TestIndexedSearchDeadlines(t *testing.T) {
	mux := http.NewServeMux()
	access := legacyWorker("node")
	access.grants[0].projects = []int64{2}
	registerIndexedSearch(mux, "search", "owner", access, func(w http.ResponseWriter, r *http.Request, project int64) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) < 4*time.Minute || !w.(*searchRecorder).deadline.After(deadline) {
			t.Error("cold search requires a bounded context and time to write its result")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	for path, token := range map[string]string{"/projects/2/search": "owner", "/internal/projects/2/search": "node"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := &searchRecorder{ResponseRecorder: httptest.NewRecorder()}
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Errorf("%s: got %d", path, w.Code)
		}
	}
}

func TestProjectHistoryAuthorizesBeforeSearch(t *testing.T) {
	worker := workerGrant{id: "alpha", digest: sha256.Sum256([]byte("scoped")), projects: []int64{2}}
	mux := http.NewServeMux()
	access := legacyWorker("legacy")
	access.grants = append(access.grants, worker)
	registerHistory(mux, nil, "owner", access)
	for _, tc := range []struct {
		token, path string
		status      int
	}{
		{"unknown", "/internal/projects/2/history", 401},
		{"scoped", "/internal/projects/4/history", 403},
		{"legacy", "/internal/projects/2/history", 403},
		{"scoped", "/projects/2/history", 401},
		{"scoped", "/internal/projects/2/history?source=all", 400},
		{"unknown", "/internal/projects/2/tasks/3/context", 401},
		{"scoped", "/internal/projects/4/tasks/3/context", 403},
		{"legacy", "/internal/projects/2/tasks/3/context", 403},
		{"scoped", "/projects/2/tasks/3/context", 401},
		{"scoped", "/internal/projects/2/tasks/0/context", 400},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d, want %d", tc.token, tc.path, w.Code, tc.status)
		}
	}
}
