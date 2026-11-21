package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkbenchPreviewAccess(t *testing.T) {
	handler := workbenchPreviewAccess(authorized("temporary-owner", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "temporary-owner")
	for _, tc := range []struct {
		method, path, peer string
		want               int
	}{
		{"GET", "/auth/session", "127.0.0.1:49810", 200},
		{"GET", "/projects/1/chat?before=3", "127.0.0.1:49810", 204},
		{"GET", "/tasks", "127.0.0.1:49810", 204},
		{"POST", "/tasks", "127.0.0.1:49810", 204},
		{"GET", "/projects", "192.0.2.4:49810", 404},
		{"POST", "/projects/clone", "127.0.0.1:49810", 404},
		{"POST", "/projects/1/chat", "127.0.0.1:49810", 404},
		{"GET", "/projects/1/git/remote", "127.0.0.1:49810", 404},
		{"POST", "/internal/claim", "127.0.0.1:49810", 404},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.RemoteAddr = tc.peer
			request.Header.Set("Authorization", "Bearer unrelated-credential")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status %d, want %d", response.Code, tc.want)
			}
		})
	}
}
