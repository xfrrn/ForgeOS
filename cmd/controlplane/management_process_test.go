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
		{"manager", `{"curation_id":1,"curation_role":"curator"}`, 403},
		{"manager", `{"test_plan_id":1,"test_plan_role":"test"}`, 403},
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

func TestCuratorProcessRouteKeepsRoleBoundary(t *testing.T) {
	t.Setenv("FORGEOS_CURATOR_PROJECT_ID", "1")
	t.Setenv("FORGEOS_CURATOR_TOKEN", "curator")
	mux := http.NewServeMux()
	if e := registerCurator(mux, nil, "owner", "manager", legacyWorker("worker")); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		token, body string
		want        int
	}{
		{"", "{}", 401}, {"manager", "{}", 401}, {"owner", "{}", 401}, {"worker", "{}", 401},
		{"curator", `{"curation_id":1,"curation_role":"management"}`, 403},
		{"curator", `{"task_id":1,"curation_role":"curator"}`, 403},
		{"curator", `{"unknown":1}`, 400}, {"curator", "{} {}", 400},
		{"curator", `{"process":{"output":{"text":"` + strings.Repeat("x", 64<<10) + `"}}}`, 400},
	} {
		req := httptest.NewRequest("POST", "/internal/curator/process", strings.NewReader(c.body))
		req.Header.Set("Authorization", "Bearer "+c.token)
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		if out.Code != c.want {
			t.Fatal(out.Code, c.want)
		}
	}
}

func TestTestAgentProcessRouteKeepsRoleBoundary(t *testing.T) {
	t.Setenv("FORGEOS_TEST_AGENT_PROJECT_ID", "1")
	t.Setenv("FORGEOS_TEST_AGENT_TOKEN", "test-agent")
	mux := http.NewServeMux()
	if e := registerTestPlans(mux, nil, "", "owner", "manager", legacyWorker("worker")); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		token, body string
		want        int
	}{
		{"", "{}", 401}, {"manager", "{}", 401}, {"owner", "{}", 401}, {"worker", "{}", 401},
		{"test-agent", `{"test_plan_id":1,"test_plan_role":"management"}`, 403},
		{"test-agent", `{"task_id":1,"test_plan_role":"test"}`, 403},
		{"test-agent", `{"unknown":1}`, 400}, {"test-agent", "{} {}", 400},
		{"test-agent", `{"process":{"output":{"text":"` + strings.Repeat("x", 64<<10) + `"}}}`, 400},
	} {
		req := httptest.NewRequest("POST", "/internal/test-agent/process", strings.NewReader(c.body))
		req.Header.Set("Authorization", "Bearer "+c.token)
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		if out.Code != c.want {
			t.Fatal(out.Code, c.want)
		}
	}
}
