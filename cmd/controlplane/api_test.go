package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// All cases must stop before reaching the database.
func TestAPIBoundaries(t *testing.T) {
	api := routes(nil, t.TempDir(), "owner", legacyWorker("worker"), nil)
	for _, tc := range []struct {
		method, path, token, body string
		status                    int
	}{
		{"POST", "/tasks", "", "{}", http.StatusUnauthorized},
		{"GET", "/tasks", "", "", http.StatusUnauthorized},
		{"GET", "/tasks", "worker", "", http.StatusUnauthorized},
		{"GET", "/projects/1/shares", "worker", "", http.StatusUnauthorized},
		{"GET", "/projects/1/shares?before=-1", "owner", "", http.StatusBadRequest},
		{"GET", "/tasks?limit=0", "owner", "", http.StatusBadRequest},
		{"GET", "/tasks?limit=101", "owner", "", http.StatusBadRequest},
		{"GET", "/tasks?before=-1", "owner", "", http.StatusBadRequest},
		{"GET", "/tasks?status=TYPO", "owner", "", http.StatusBadRequest},
		{"GET", "/tasks?q=%00", "owner", "", http.StatusBadRequest},
		{"GET", "/tasks/1", "worker", "", http.StatusUnauthorized},
		{"POST", "/internal/claim", "owner", "{}", http.StatusUnauthorized},
		{"POST", "/executions/1/abandon", "worker", "", http.StatusUnauthorized},
		{"POST", "/executions/1/cancel", "worker", "", http.StatusUnauthorized},
		{"POST", "/executions/1/instructions", "worker", "{}", http.StatusUnauthorized},
		{"POST", "/executions/1/instructions", "owner", "{}", http.StatusBadRequest},
		{"POST", "/executions/1/pause-ci", "worker", "", http.StatusUnauthorized},
		{"POST", "/executions/1/resume-ci", "worker", "", http.StatusUnauthorized},
		{"POST", "/internal/executions/1/ci-candidate", "owner", "", http.StatusUnauthorized},
		{"POST", "/executions/1/retry", "worker", "", http.StatusUnauthorized},
		{"POST", "/tasks/1/acceptance", "worker", "{}", http.StatusUnauthorized},
		{"POST", "/tasks/1/acceptance", "owner", `{"decision":"approve","commit":"HEAD"}`, http.StatusBadRequest},
		{"POST", "/tasks/1/acceptance", "owner", `{"execution_id":1,"decision":"reject","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","note":" "}`, http.StatusBadRequest},
		{"POST", "/tasks", "owner", `{"project_id":1,"kind":"shell"}`, http.StatusBadRequest},
		{"POST", "/tasks", "owner", `{"project_id":1,"command":"anything"}`, http.StatusBadRequest},
		{"POST", "/tasks", "owner", `{"project_id":1}{}`, http.StatusBadRequest},
		{"POST", "/projects", "owner", `{"path":"../missing"}`, http.StatusBadRequest},
		{"GET", "/tasks/0", "owner", "", http.StatusBadRequest},
		{"POST", "/internal/claim", "worker", `{"worker":"w","workspace_root":"relative"}`, http.StatusBadRequest},
		{"POST", "/internal/executions/1/result", "worker", `{"lease_token":"x","commit":"invalid","exit_code":0}`, http.StatusBadRequest},
		{"POST", "/internal/executions/1/result", "worker", `{"lease_token":"x"}`, http.StatusBadRequest},
		{"POST", "/internal/executions/1/result", "worker", `{"lease_token":"x","exit_code":0,"commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","validation":{"profile":"docs","exit_code":null}}`, http.StatusBadRequest},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		resp := httptest.NewRecorder()
		api.ServeHTTP(resp, req)
		if resp.Code != tc.status {
			t.Errorf("%s %s (%s) = %d, want %d", tc.method, tc.path, tc.body, resp.Code, tc.status)
		}
	}
}
