package main

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func routes(data *store.Store, root, owner string, workers workerAccess, delivery deliveryPolicy) *http.ServeMux {
	mux := http.NewServeMux()
	registerCheckpoints(mux, data, owner, workers)
	registerHistory(mux, data, owner, workers)
	registerValidationPlan(mux, data, root, owner, workers)
	registerDecisions(mux, data, owner, workers)
	registerMaterials(mux, data, owner, workers)
	registerShares(mux, data, owner, workers)
	registerIssueRoute(mux, data, root, owner)
	registerPlanRoutes(mux, data, root, owner)
	registerQuestionRoutes(mux, data, owner)
	registerWebRoutes(mux, data, root, owner)
	mux.HandleFunc("POST /tasks/{id}/delivery/refresh", authorized(owner, delivery.refresh(data, root)))
	mux.HandleFunc("POST /tasks/{id}/acceptance", authorized(owner, delivery.review(data)))
	mux.HandleFunc("GET /tasks", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := store.TaskListFilter{Status: q.Get("status"), Query: strings.TrimSpace(q.Get("q"))}
		if filter.Status == "ALL" {
			filter.Status = ""
		}
		if (filter.Status != "" && !store.ValidTaskStatus(filter.Status)) ||
			strings.ContainsRune(filter.Query, 0) || !utf8.ValidString(filter.Query) {
			problem(w, http.StatusBadRequest, "invalid task status or search text")
			return
		}
		var err error
		if q.Has("limit") {
			filter.Limit, err = strconv.Atoi(q.Get("limit"))
			if err != nil || filter.Limit < 1 || filter.Limit > 100 {
				problem(w, http.StatusBadRequest, "limit must be between 1 and 100")
				return
			}
		}
		if q.Has("before") {
			filter.Before, err = strconv.ParseInt(q.Get("before"), 10, 64)
			if err != nil || filter.Before < 1 {
				problem(w, http.StatusBadRequest, "before must be a positive task ID")
				return
			}
		}
		page, err := data.ListTasks(r.Context(), filter)
		respond(w, http.StatusOK, page, err)
	}))

	mux.HandleFunc("POST /projects", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Path string `json:"path"`
		}
		if !decode(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.Path) == "" {
			problem(w, http.StatusBadRequest, "path is required")
			return
		}
		path, err := localrepo.ResolveRepo(r.Context(), root, input.Path)
		if err != nil {
			problem(w, http.StatusBadRequest, "path must identify a Git repository inside FORGEOS_REPO_ROOT")
			return
		}
		project, err := data.CreateProject(r.Context(), path)
		respond(w, http.StatusOK, project, err)
	}))

	mux.HandleFunc("POST /tasks", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID int64  `json:"project_id"`
			Kind      string `json:"kind"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.ProjectID <= 0 || (input.Kind != "" && input.Kind != "git_head") {
			problem(w, http.StatusBadRequest, "provide a positive project_id and kind git_head")
			return
		}
		task, err := data.CreateTask(r.Context(), input.ProjectID)
		respond(w, http.StatusCreated, task, err)
	}))

	mux.HandleFunc("GET /tasks/{id}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		task, err := data.Task(r.Context(), id)
		respond(w, http.StatusOK, task, err)
	}))

	mux.HandleFunc("POST /executions/{id}/abandon", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		err := data.Abandon(r.Context(), id)
		respond(w, http.StatusNoContent, nil, err)
	}))

	mux.HandleFunc("POST /executions/{id}/retry", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		task, err := data.RetryIssueExecution(r.Context(), id)
		respond(w, http.StatusOK, task, err)
	}))

	mux.HandleFunc("POST /internal/claim", workers.authorize(func(w http.ResponseWriter, r *http.Request, grant *workerGrant) {
		var input struct {
			Worker                 string `json:"worker"`
			WorkspaceRoot          string `json:"workspace_root"`
			SupportsValidation     bool   `json:"supports_validation"`
			SupportsDeclaredChecks bool   `json:"supports_declared_checks"`
			SupportsCheckpoints    bool   `json:"supports_checkpoints"`
			SupportsCodex          bool   `json:"supports_codex"`
			SupportsRework         bool   `json:"supports_rework"`
			SupportsPlanner        bool   `json:"supports_planner"`
		}
		if !decode(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.Worker) == "" || len(input.Worker) > 128 || !filepath.IsAbs(input.WorkspaceRoot) {
			problem(w, http.StatusBadRequest, "provide a worker identity and absolute workspace_root")
			return
		}
		if grant.id != "" && input.Worker != grant.id {
			problem(w, http.StatusForbidden, "claim does not match the authenticated Worker")
			return
		}
		job, err := data.ClaimForProjects(r.Context(), input.Worker, input.WorkspaceRoot, grant.projects, input.SupportsCodex, input.SupportsRework, input.SupportsPlanner, input.SupportsCheckpoints, input.SupportsValidation, input.SupportsDeclaredChecks)
		if job == nil && err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		respond(w, http.StatusOK, job, err)
	}))

	mux.HandleFunc("POST /internal/executions/{id}/result", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		var input struct {
			LeaseToken string      `json:"lease_token"`
			Plan       *store.Plan `json:"plan"`
			Commit     string      `json:"commit"`
			ExitCode   *int        `json:"exit_code"`
			Error      string      `json:"error"`
			BaseCommit string      `json:"base_commit"`
			SessionID  string      `json:"session_id"`
			Validation *struct {
				Profile  string          `json:"profile"`
				Commit   string          `json:"commit"`
				ExitCode *int            `json:"exit_code"`
				Output   string          `json:"output"`
				Run      *validation.Run `json:"run"`
			} `json:"validation"`
		}
		if !decodeLimited(w, r, &input, 4<<20) {
			return
		}
		if input.LeaseToken == "" || input.ExitCode == nil || *input.ExitCode < -1 || *input.ExitCode > 2147483647 || len(input.Error) > 1024 {
			problem(w, http.StatusBadRequest, "invalid lease or result")
			return
		}
		sha, err := hex.DecodeString(input.Commit)
		validSHA := err == nil && (len(sha) == 20 || len(sha) == 32)
		if (*input.ExitCode == 0 && ((!validSHA && input.Plan == nil) || input.Error != "")) ||
			(*input.ExitCode != 0 && (strings.TrimSpace(input.Error) == "" || (input.Commit != "" && !validSHA))) {
			problem(w, http.StatusBadRequest, "success requires a commit, failure requires an error")
			return
		}
		if len(input.SessionID) > 256 || len(input.BaseCommit) > 64 || (input.Validation != nil && len(input.Validation.Output) > 2048) {
			problem(w, http.StatusBadRequest, "execution evidence exceeds the allowed length")
			return
		}
		result := store.Result{Commit: input.Commit, ExitCode: *input.ExitCode, Error: input.Error,
			BaseCommit: input.BaseCommit, SessionID: input.SessionID, Plan: input.Plan}
		if v := input.Validation; v != nil {
			if v.ExitCode == nil || *v.ExitCode < -1 || *v.ExitCode > 2147483647 {
				problem(w, http.StatusBadRequest, "validation requires an explicit exit code")
				return
			}
			result.Validation = &store.ValidationResult{Profile: v.Profile, Commit: v.Commit, ExitCode: *v.ExitCode, Output: v.Output, Run: v.Run}
		}
		err = data.CompleteWithChecks(r.Context(), id, input.LeaseToken, result, verifyDeclaredChecks(root))
		respond(w, http.StatusNoContent, nil, err)
	}))
	return mux
}

func authorized(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			problem(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		next(w, r.WithContext(ctx))
	}
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		problem(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	return decodeLimited(w, r, dest, 16<<10)
}

func decodeLimited(w http.ResponseWriter, r *http.Request, dest any, limit int64) bool {
	reader := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	reader.DisallowUnknownFields()
	if err := reader.Decode(dest); err != nil || reader.Decode(new(any)) != io.EOF {
		problem(w, http.StatusBadRequest, "expected a single JSON object with supported fields")
		return false
	}
	return true
}

func respond(w http.ResponseWriter, status int, value any, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		problem(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, store.ErrConflict):
		problem(w, http.StatusConflict, "request conflicts with the current task state or recorded decision")
	case err != nil:
		log.Printf("control plane request failed: %v", err)
		problem(w, http.StatusInternalServerError, "database operation failed")
	case status == http.StatusNoContent:
		w.WriteHeader(status)
	default:
		writeJSON(w, status, value)
	}
}

func problem(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
