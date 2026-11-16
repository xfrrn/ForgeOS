package main

import (
	"errors"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

func registerTestPlans(mux *http.ServeMux, data *store.Store, root, owner, manager string, workers *workerRegistry) error {
	record := func(w http.ResponseWriter, r *http.Request, task, project int64, tester, actor string) {
		var body store.TestCaseResult
		if !decodeLimited(w, r, &body, 128<<10) {
			return
		}
		receipt, err := data.RecordTestCase(r.Context(), task, project, tester, actor, body)
		respond(w, 200, receipt, err)
	}
	mux.HandleFunc("POST /tasks/{id}/test-results", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			record(w, r, id, 0, "human", "owner")
		}
	}))
	workers.handle(mux, "POST /internal/projects/{project}/tasks/{id}/test-results", workers.authorize(func(w http.ResponseWriter, r *http.Request, grant *workerGrant) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		project, err := strconv.ParseInt(r.PathValue("project"), 10, 64)
		if err != nil || project < 1 {
			problem(w, 400, "invalid project")
			return
		}
		if grant.id == "" || !slices.Contains(grant.projects, project) {
			problem(w, 403, "project-scoped runner required")
			return
		}
		record(w, r, id, project, "runner", grant.id)
	}))
	mux.HandleFunc("POST /tasks/{id}/test-plans", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var req struct {
			Execution int64    `json:"execution_id"`
			Commit    string   `json:"commit"`
			Previous  int      `json:"previous_revision"`
			Criteria  []string `json:"criteria"`
		}
		if !ok || !decodeLimited(w, r, &req, 256<<10) {
			return
		}
		for _, c := range req.Criteria {
			if strings.TrimSpace(c) == "" || len(c) > 16<<10 {
				problem(w, 400, "criteria must be nonempty and at most 16 KiB each")
				return
			}
		}
		receipt, err := data.RequestTestPlan(r.Context(), root, id, req.Execution, req.Commit, req.Previous, req.Criteria)
		respond(w, 200, receipt, err)
	}))
	mux.HandleFunc("GET /tasks/{id}/test-plans", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		execution, err := strconv.ParseInt(r.URL.Query().Get("execution_id"), 10, 64)
		if err != nil || execution < 1 {
			problem(w, 400, "provide a positive execution_id")
			return
		}
		rev := 0
		if s := r.URL.Query().Get("revision"); s != "" {
			rev, err = strconv.Atoi(s)
			if err != nil || rev < 1 {
				problem(w, 400, "provide a positive revision")
				return
			}
		}
		receipt, err := data.TestPlanReceipt(r.Context(), id, execution, rev)
		respond(w, 200, receipt, err)
	}))
	scope, token := os.Getenv("FORGEOS_TEST_AGENT_PROJECT_ID"), os.Getenv("FORGEOS_TEST_AGENT_TOKEN")
	if scope == "" && token == "" {
		return nil
	}
	project, err := strconv.ParseInt(scope, 10, 64)
	if err != nil || project < 1 || strings.TrimSpace(token) == "" || token == owner || token == manager || token == os.Getenv("FORGEOS_REVIEWER_TOKEN") || workers.match(token) != nil {
		return errors.New("set FORGEOS_TEST_AGENT_PROJECT_ID and a distinct FORGEOS_TEST_AGENT_TOKEN")
	}
	mux.HandleFunc("POST /internal/test-agent/claim", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		job, err := data.ClaimTestPlan(r.Context(), project, "test")
		if job == nil && err == nil {
			w.WriteHeader(204)
			return
		}
		respond(w, 200, job, err)
	}))
	registerTestPlanResult(mux, "/internal/test-agent/result", data, token, project, "test")
	return nil
}

func registerTestPlanResult(mux *http.ServeMux, path string, data *store.Store, token string, project int64, role string) {
	mux.HandleFunc("POST "+path, authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var out store.TestPlanOutcome
		if !decodeLimited(w, r, &out, 2<<20) {
			return
		}
		respond(w, 204, nil, data.FinishTestPlan(r.Context(), project, role, out))
	}))
}
