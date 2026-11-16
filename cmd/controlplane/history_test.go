package main

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectHistoryAuthorizesBeforeSearch(t *testing.T) {
	worker := workerGrant{id: "alpha", digest: sha256.Sum256([]byte("scoped")), projects: []int64{2}}
	mux := http.NewServeMux()
	registerHistory(mux, nil, "owner", append(legacyWorker("legacy"), worker))
	for _, tc := range []struct {
		token, path string
		status      int
	}{
		{"unknown", "/internal/projects/2/history", 401},
		{"scoped", "/internal/projects/4/history", 403},
		{"legacy", "/internal/projects/2/history", 403},
		{"scoped", "/projects/2/history", 401},
		{"scoped", "/internal/projects/2/history?source=all", 400},
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
