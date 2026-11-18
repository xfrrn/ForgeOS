package main

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/review"
	"forgeos/internal/store"
)

func registerCodeReviews(mux *http.ServeMux, data *store.Store, root, owner, manager string, workers *workerRegistry) error {
	mux.HandleFunc("PUT /projects/{id}/review-policy", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var input struct {
			Required *bool `json:"required"`
		}
		if !ok || !decode(w, r, &input) {
			return
		}
		if input.Required == nil {
			problem(w, 400, "required must be a boolean")
			return
		}
		respond(w, 204, nil, data.SetCodeReviewPolicy(r.Context(), id, *input.Required))
	}))
	mux.HandleFunc("POST /tasks/{id}/code-review", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var input struct {
			Execution int64  `json:"execution_id"`
			Commit    string `json:"commit"`
		}
		if !ok || !decode(w, r, &input) {
			return
		}
		respond(w, 204, nil, data.RequestCodeReview(r.Context(), id, input.Execution, input.Commit))
	}))
	mux.HandleFunc("GET /tasks/{id}/code-review", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		execution, err := strconv.ParseInt(r.URL.Query().Get("execution_id"), 10, 64)
		if err != nil || execution < 1 {
			problem(w, 400, "provide a positive execution_id")
			return
		}
		receipt, err := data.ReviewReceipt(r.Context(), id, execution)
		respond(w, 200, receipt, err)
	}))
	mux.HandleFunc("POST /tasks/{id}/code-review/check", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var input struct {
			Execution int64  `json:"execution_id"`
			Commit    string `json:"commit"`
			Note      string `json:"note"`
		}
		if !ok || !decode(w, r, &input) {
			return
		}
		respond(w, 204, nil, data.CheckReviewFindings(r.Context(), id, input.Execution, input.Commit, input.Note))
	}))
	scope, token := os.Getenv("FORGEOS_REVIEWER_PROJECT_ID"), os.Getenv("FORGEOS_REVIEWER_TOKEN")
	if scope == "" && token == "" {
		return nil
	}
	project, err := strconv.ParseInt(scope, 10, 64)
	if err != nil || project < 1 || strings.TrimSpace(token) == "" || token == owner || token == manager || workers.match(token) != nil {
		return errors.New("set FORGEOS_REVIEWER_PROJECT_ID and a distinct FORGEOS_REVIEWER_TOKEN")
	}
	mux.HandleFunc("POST /internal/reviewer/claim", authorized(token, idempotentRoleClaim(data, project, "reviewer", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			SupportsContext bool `json:"supports_candidate_context"`
		}
		if r.ContentLength != 0 && !decode(w, r, &input) {
			return
		}
		job, err := data.ClaimCodeReview(r.Context(), project, input.SupportsContext, r.URL.Query().Get("supports_budget_deadline") == "1")
		if err == nil && job == nil {
			w.WriteHeader(204)
			return
		}
		if job != nil {
			job.CaptureProcess = r.URL.Query().Get("supports_process") == "1"
			job.RepoRelativePath = localrepo.RelativePath(root, job.Repo)
		}
		respond(w, 200, job, err)
	})))
	mux.HandleFunc("POST /internal/reviewer/process", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var report store.ManagementProcessReport
		if decodeLimited(w, r, &report, 64<<10) {
			if report.ReviewerExecution <= 0 {
				problem(w, 403, "Reviewer identity only reports candidate review processes")
				return
			}
			respond(w, 204, nil, data.ObserveManagementProcess(r.Context(), project, report))
		}
	}))
	mux.HandleFunc("POST /internal/reviewer/result", authorized(token, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Execution int64          `json:"execution_id"`
			Lease     string         `json:"lease"`
			Report    *review.Report `json:"report"`
			Failed    bool           `json:"failed"`
		}
		if !decodeLimited(w, r, &input, 512<<10) {
			return
		}
		if input.Failed == (input.Report != nil) {
			problem(w, 400, "provide one review outcome")
			return
		}
		ctx := r.Context()
		if input.Failed {
			respond(w, 204, nil, data.FailCodeReview(ctx, project, input.Execution, input.Lease))
			return
		}
		job, err := data.CodeReviewTarget(ctx, project, input.Execution, input.Lease)
		if err != nil {
			respond(w, 204, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(ctx, root, job.Repo)
		if err != nil {
			problem(w, 503, "review repository unavailable")
			return
		}
		snapshot, err := review.Read(ctx, repo, job.Base, job.Commit)
		if err != nil {
			problem(w, 409, "candidate snapshot unavailable for evidence verification")
			return
		}
		if err = snapshot.Verify(input.Report); err != nil {
			problem(w, 400, "invalid candidate review report")
			return
		}
		respond(w, 204, nil, data.CompleteCodeReview(ctx, project, input.Execution, input.Lease, *input.Report))
	}))
	return nil
}
