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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func routes(data *store.Store, root, owner string, workers *workerRegistry, delivery deliveryTargets) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /workers/reload-credentials", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		if err := workers.reload(); err != nil {
			problem(w, http.StatusConflict, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /workers", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		nodes, err := data.WorkerNodes(r.Context())
		respond(w, http.StatusOK, nodes, err)
	}))
	workers.handle(mux, "POST /internal/workers/heartbeat", workers.authorize(func(w http.ResponseWriter, r *http.Request, grant *workerGrant) {
		var input store.WorkerHeartbeat
		if !decode(w, r, &input) {
			return
		}
		if !input.Valid() {
			problem(w, 400, "invalid Worker identity, platform or capacity")
			return
		}
		if grant.id != "" && input.Worker != grant.id {
			problem(w, 403, "heartbeat does not match the authenticated Worker")
			return
		}
		respond(w, http.StatusNoContent, nil, data.RecordHeartbeat(r.Context(), input))
	}))
	registerCheckpoints(mux, data, owner, workers)
	registerHistory(mux, data, owner, workers)
	registerRepositoryInventory(mux, data, root, owner)
	registerDocumentChanges(mux, data, root, owner)
	registerInitializationReports(mux, data, owner)
	registerValidationPlan(mux, data, root, owner, workers)
	registerCIFiles(mux, data, owner, workers)
	registerDecisions(mux, data, owner, workers)
	registerMaterials(mux, data, owner, workers)
	registerGlobalRules(mux, data, owner, workers)
	registerCurationReads(mux, data, owner, workers)
	registerShares(mux, data, owner, workers)
	registerIssueRoute(mux, data, root, owner)
	registerPlanRoutes(mux, data, root, owner)
	registerQuestionRoutes(mux, data, owner)
	registerNotifications(mux, data, owner)
	registerApprovals(mux, data, owner)
	registerWebRoutes(mux, data, root, owner)
	registerPreviewFeedback(mux, data, owner)
	mux.HandleFunc("POST /tasks/{id}/delivery/refresh", authorized(owner, delivery.refresh(data, root)))
	mux.HandleFunc("POST /tasks/{id}/acceptance", authorized(owner, delivery.review(data)))
	mux.HandleFunc("POST /tasks/{id}/validation-status", authorized(owner, delivery.validationStatus(data, root)))
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

	mux.HandleFunc("GET /projects", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var before int64
		if q := r.URL.Query(); q.Has("before") {
			var err error
			if before, err = strconv.ParseInt(q.Get("before"), 10, 64); err != nil || before <= 0 {
				problem(w, 400, "before must be a positive project ID")
				return
			}
		}
		rows, err := data.RegisteredProjects(r.Context(), before)
		var next int64
		if len(rows) > 30 {
			rows = rows[:30]
			next = rows[len(rows)-1].ID
		}
		respond(w, 200, struct {
			Items []store.RegisteredProject `json:"items"`
			Next  int64                     `json:"next_before,omitempty"`
		}{rows, next}, err)
	}))
	mux.HandleFunc("POST /projects/{id}/check", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			Retry int64 `json:"retry_task_id"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Retry < 0 {
			problem(w, 400, "retry_task_id must be positive")
			return
		}
		task, err := data.ProjectCheck(r.Context(), id, input.Retry)
		if err == nil {
			scan, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if scanErr := observeDocuments(scan, data, root, id); scanErr != nil {
				log.Printf("project %d document scan at HEAD check: %v", id, scanErr)
			}
		}
		respond(w, 200, task, err)
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

	for _, action := range []string{"pause-ci", "resume-ci"} {
		mux.HandleFunc("POST /executions/{id}/"+action, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			if id, ok := pathID(w, r); ok {
				respond(w, http.StatusNoContent, nil, data.ControlCI(r.Context(), id, action == "resume-ci"))
			}
		}))
	}
	workers.handle(mux, "POST /internal/executions/{id}/ci-candidate", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var input struct {
			Lease   string `json:"lease_token"`
			Commit  string `json:"commit"`
			Session string `json:"session_id"`
		}
		if ok && decode(w, r, &input) {
			respond(w, http.StatusNoContent, nil, data.RecordCICandidate(r.Context(), id, input.Lease, input.Commit, input.Session))
		}
	}))

	mux.HandleFunc("POST /executions/{id}/cancel", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if valid {
			respond(w, http.StatusNoContent, nil, data.CancelExecution(r.Context(), id))
		}
	}))

	mux.HandleFunc("POST /executions/{id}/retry", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		task, err := data.RetryIssueExecution(r.Context(), id)
		respond(w, http.StatusOK, task, err)
	}))

	workers.handle(mux, "POST /internal/claim", workers.authorize(func(w http.ResponseWriter, r *http.Request, grant *workerGrant) {
		var input struct {
			Worker        string `json:"worker"`
			WorkspaceRoot string `json:"workspace_root"`
			store.WorkerCapabilities
		}
		if !decode(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.Worker) == "" || len(input.Worker) > 128 || !store.ValidWorkspaceRoot(input.WorkspaceRoot) {
			problem(w, http.StatusBadRequest, "provide a worker identity and absolute workspace_root")
			return
		}
		if grant.id != "" && input.Worker != grant.id {
			problem(w, http.StatusForbidden, "claim does not match the authenticated Worker")
			return
		}
		job, err := data.ClaimForProjects(r.Context(), input.Worker, input.WorkspaceRoot, grant.projects, input.WorkerCapabilities)
		if job == nil && err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if job != nil {
			job.RepoRelativePath = localrepo.RelativePath(root, job.RepoPath)
		}
		respond(w, http.StatusOK, job, err)
	}))

	workers.handle(mux, "POST /internal/executions/{id}/resource", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var input store.ResourceRequest
		if !ok || !decode(w, r, &input) {
			return
		}
		granted, err := data.ReserveExecutionResource(r.Context(), id, input)
		if err == nil && !granted {
			problem(w, http.StatusTooManyRequests, "node resource capacity is in use")
			return
		}
		respond(w, http.StatusNoContent, nil, err)
	}))
	workers.handle(mux, "POST /internal/executions/{id}/process", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var report store.ProcessReport
		if !ok || !decode(w, r, &report) {
			return
		}
		if len(report.LeaseToken) != 64 || !report.Process.Valid() {
			problem(w, 400, "invalid execution process report")
			return
		}
		respond(w, 204, nil, data.ObserveProcess(r.Context(), id, report))
	}))
	workers.handle(mux, "POST /internal/executions/{id}/renew", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			LeaseToken string `json:"lease_token"`
		}
		if !decode(w, r, &input) {
			return
		}
		if len(input.LeaseToken) != 64 {
			problem(w, http.StatusBadRequest, "provide the execution lease token")
			return
		}
		respond(w, http.StatusNoContent, nil, data.RenewExecutionLease(r.Context(), id, input.LeaseToken))
	}))

	workers.handle(mux, "POST /internal/executions/{id}/result", workers.execution(data, func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		var input struct {
			LeaseToken     string                `json:"lease_token"`
			EnvironmentRun *store.EnvironmentRun `json:"environment_run"`
			Processes      *store.ProcessHistory `json:"processes"`
			Plan           *store.Plan           `json:"plan"`
			Commit         string                `json:"commit"`
			ExitCode       *int                  `json:"exit_code"`
			Error          string                `json:"error"`
			BaseCommit     string                `json:"base_commit"`
			SessionID      string                `json:"session_id"`
			Harness        string                `json:"harness"`
			HarnessVersion string                `json:"harness_version"`
			Validation     *struct {
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
		result := store.Result{EnvironmentRun: input.EnvironmentRun, Commit: input.Commit, ExitCode: *input.ExitCode, Error: input.Error, Harness: input.Harness, HarnessVersion: input.HarnessVersion,
			BaseCommit: input.BaseCommit, SessionID: input.SessionID, Plan: input.Plan, Processes: input.Processes}
		if v := input.Validation; v != nil {
			if v.ExitCode == nil || *v.ExitCode < -1 || *v.ExitCode > 2147483647 {
				problem(w, http.StatusBadRequest, "validation requires an explicit exit code")
				return
			}
			result.Validation = &store.ValidationResult{Profile: v.Profile, Commit: v.Commit, ExitCode: *v.ExitCode, Output: v.Output, Run: v.Run}
		}
		err = data.CompleteWithChecks(r.Context(), id, input.LeaseToken, result, verifyDeclaredChecks(root))
		if err == nil && input.Validation != nil && input.Validation.Run != nil {
			w.Header().Set("ForgeOS-CI-Archive", "1")
		}
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
	var expiry store.RetentionExpiry
	switch {
	case errors.Is(err, store.ErrNotFound):
		problem(w, http.StatusNotFound, "resource not found")
	case errors.As(err, &expiry):
		writeJSON(w, http.StatusGone, struct {
			Error    string `json:"error"`
			Revision int64  `json:"retention_revision,omitempty"`
		}{expiry.Error(), expiry.Revision})
	case errors.Is(err, store.ErrOwnerStop):
		problem(w, http.StatusLocked, err.Error())
	case errors.Is(err, store.ErrSourceClosed):
		problem(w, http.StatusGone, err.Error())
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
