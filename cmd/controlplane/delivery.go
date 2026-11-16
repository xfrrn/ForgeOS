package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

type deliveryPolicy struct {
	projectID int64
	target    string
}

func deliveryConfig(id, target string) (deliveryPolicy, error) {
	if id == "" && target == "" {
		return deliveryPolicy{}, nil
	}
	projectID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || projectID <= 0 {
		return deliveryPolicy{}, errors.New("set FORGEOS_DELIVERY_PROJECT_ID to the project authorized for GitHub writes")
	}
	if target != "" {
		if strings.HasPrefix(target, "-") || target == "HEAD" || exec.Command("git", "check-ref-format", "refs/heads/"+target).Run() != nil {
			return deliveryPolicy{}, errors.New("invalid FORGEOS_DELIVERY_TARGET_BRANCH")
		}
	}
	return deliveryPolicy{projectID: projectID, target: target}, nil
}

func (p deliveryPolicy) review(data *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		var input struct {
			ExecutionID int64  `json:"execution_id"`
			Commit      string `json:"commit"`
			Decision    string `json:"decision"`
			Note        string `json:"note"`
		}
		if !decode(w, r, &input) {
			return
		}
		input.Note = strings.TrimSpace(input.Note)
		if input.ExecutionID <= 0 || !gitCommit.MatchString(input.Commit) || (input.Decision != "approve" && input.Decision != "reject") ||
			(input.Decision == "reject" && input.Note == "") || len(input.Note) > 4096 || strings.ContainsRune(input.Note, 0) {
			problem(w, http.StatusBadRequest, "provide execution_id, the full candidate commit and approve or reject; rejection requires a note (up to 4096 bytes)")
			return
		}
		publish := false
		if input.Decision == "approve" && p.projectID != 0 {
			task, err := data.Task(r.Context(), id)
			if err != nil {
				respond(w, 0, nil, err)
				return
			}
			publish = task.ProjectID == p.projectID
		}
		task, err := data.Review(r.Context(), id, input.ExecutionID, input.Commit, input.Decision, input.Note, publish, p.target)
		respond(w, http.StatusOK, task, err)
	}
}

type advanceDelivery func(context.Context, string, store.Task, store.Delivery) (store.Delivery, error)

func (p deliveryPolicy) step(ctx context.Context, data *store.Store, root string, advance advanceDelivery) (bool, error) {
	job, err := data.ClaimDelivery(ctx, p.projectID)
	if err != nil || job == nil {
		return false, err
	}
	task, err := data.Task(ctx, job.TaskID)
	if err != nil {
		return true, err
	}
	project, err := data.Project(ctx, task.ProjectID)
	if err != nil {
		return true, err
	}
	next := *job
	if project.ID != p.projectID || task.Spec == nil || task.Status != "ACCEPTED" ||
		task.Execution == nil || task.Execution.ID != job.ExecutionID || task.Execution.Result == nil ||
		task.Execution.Result.Commit != job.Commit || task.Spec.Issue.RepositoryID != project.RepositoryID {
		next.Detail = "delivery does not match the accepted task or the configured project"
	} else if repo, err := localrepo.ResolveRepo(ctx, root, project.Path); err != nil {
		next.Detail = "registered project repository is unavailable"
	} else {
		updated, err := advance(ctx, repo, task, *job)
		if err == nil {
			next = updated
		} else {
			// A timed-out remote call may already have succeeded. Reconcile it before retrying.
			next.Detail = err.Error()
		}
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	return true, data.SaveDelivery(ctx, next)
}

func (p deliveryPolicy) start(parent context.Context, data *store.Store, root string) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var lastDependent int64
		for ctx.Err() == nil {
			// The remote call finishes before the database claim expires.
			stepCtx, stop := context.WithTimeout(ctx, 75*time.Second)
			claimed, err := p.step(stepCtx, data, root, advanceGitHub)
			if err == nil && !claimed {
				claimed, err = p.releaseDependent(stepCtx, data, root, &lastDependent)
			}
			stop()
			if err != nil && ctx.Err() == nil {
				log.Printf("delivery for project %d: %v", p.projectID, err)
			}
			if claimed && err == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}()
	return func() { cancel(); <-done }
}
