package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagementProcessRouteKeepsRoleAndBodyBoundary(t *testing.T) {
	mux := http.NewServeMux()
	if err := registerManagement(mux, nil, "1", "manager", "owner", legacyWorker("worker")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		token, body string
		want        int
	}{
		{"", "{}", 401}, {"owner", "{}", 401}, {"worker", "{}", 401},
		{"manager", `{"unexpected":1}`, 400}, {"manager", "{} {}", 400},
		{"manager", `{"process":{"output":{"text":"` + strings.Repeat("x", 64<<10) + `"}}}`, 400},
	} {
		req := httptest.NewRequest("POST", "/internal/management/process", strings.NewReader(c.body))
		req.Header.Set("Authorization", "Bearer "+c.token)
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		if out.Code != c.want {
			t.Fatal(out.Code, c.want)
		}
	}
}
