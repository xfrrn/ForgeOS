package main

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func verifyDeclaredChecks(root string) store.CheckVerifier {
	return func(ctx context.Context, project string, spec store.IssueSpec, result store.Result) (bool, error) {
		repo, err := localrepo.ResolveRepo(ctx, root, project)
		if err != nil {
			return false, errors.New("validation repository unavailable")
		}
		level := spec.Validation
		if level == "auto" {
			if spec.Policy == nil || result.Validation == nil || result.Validation.Run == nil {
				return false, store.ErrConflict
			}
			decision, err := validation.Assess(ctx, repo, spec.BaseCommit, result.Commit, *spec.Policy)
			if err != nil || !reflect.DeepEqual(&decision, result.Validation.Run.Decision) {
				return false, store.ErrConflict
			}
			level = decision.Level
		}
		plan, err := validation.ReadPlan(ctx, repo, spec.BaseCommit, level)
		if err != nil || !plan.Accepts(result.Validation.Run) {
			return false, store.ErrConflict
		}
		out, err := localrepo.Git(ctx, repo, "rev-list", "-1", "--parents", result.Commit, "--")
		if err != nil || strings.TrimSpace(string(out)) != result.Commit+" "+spec.BaseCommit {
			return false, store.ErrConflict
		}
		changes, err := localrepo.Git(ctx, repo, "diff", "--no-ext-diff", "--no-textconv", "--name-only", "-z", spec.BaseCommit, result.Commit, "--", "validation.yaml", "acceptance.yaml")
		return len(changes) > 0, err
	}
}

func registerValidationPlan(mux *http.ServeMux, data *store.Store, root, owner string, workers workerAccess) {
	registerProjectRead(mux, "validation-assessment", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		project, err := data.Project(r.Context(), id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(r.Context(), root, project.Path)
		if err != nil {
			problem(w, 409, "registered repository unavailable")
			return
		}
		q := r.URL.Query()
		d, err := validation.Assess(r.Context(), repo, q.Get("base"), q.Get("commit"), validation.Policy{Minimum: q.Get("minimum"), TaskType: q.Get("task_type"), Target: q.Get("target"), Management: q.Get("management_minimum")})
		if err != nil {
			problem(w, 422, "cannot assess risk for the supplied revisions, policy or acceptance.yaml")
			return
		}
		respond(w, 200, d, nil)
	})
	registerProjectRead(mux, "validation-plan", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		project, err := data.Project(r.Context(), id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(r.Context(), root, project.Path)
		if err != nil {
			problem(w, 409, "registered repository is unavailable")
			return
		}
		q := r.URL.Query()
		plan, err := validation.ReadPlan(r.Context(), repo, q.Get("commit"), q.Get("level"))
		if err != nil {
			detail := err.Error()
			if len(detail) > 2048 {
				detail = "validation configuration does not satisfy the schema"
			}
			problem(w, http.StatusUnprocessableEntity, detail)
			return
		}
		respond(w, 200, plan, nil)
	})
}
