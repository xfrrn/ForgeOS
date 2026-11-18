package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func registerCIFiles(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	registerPolicy := func(path string, update func(context.Context, int64, *store.CIRetention) (store.CIRetention, error)) {
		policy := authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			id, ok := pathID(w, r)
			if !ok {
				return
			}
			var replacement *store.CIRetention
			if r.Method == http.MethodPut {
				if !decode(w, r, &replacement) {
					return
				}
				if replacement == nil || replacement.LogSeconds < 0 || replacement.ReportSeconds < 0 || replacement.ArtifactSeconds < 0 {
					problem(w, 400, "provide a retention object with nonnegative seconds")
					return
				}
			}
			current, err := update(r.Context(), id, replacement)
			respond(w, 200, current, err)
		})
		mux.HandleFunc("GET "+path, policy)
		mux.HandleFunc("PUT "+path, policy)
	}
	registerPolicy("/projects/{id}/ci-retention", data.ProjectCIRetention)
	registerPolicy("/projects/{id}/local-ci-retention", data.ProjectLocalCIRetention)
	workers.handle(mux, "POST /internal/workers/ci-cleanup", workers.authorize(func(w http.ResponseWriter, r *http.Request, grant *workerGrant) {
		var input struct {
			Worker    string `json:"worker"`
			After     int64  `json:"after_execution"`
			Reconcile bool   `json:"reconcile"`
			File      string `json:"after_file"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Worker == "" || input.After < 0 || len(input.File) > 96 {
			problem(w, 400, "invalid Worker or cleanup cursor")
			return
		}
		if grant.id != "" && grant.id != input.Worker {
			problem(w, 403, "cleanup belongs to another Worker")
			return
		}
		files, err := data.LocalCICleanup(r.Context(), input.Worker, grant.projects, input.After, input.File, input.Reconcile)
		respond(w, 200, files, err)
	}))
	workers.handle(mux, "POST /internal/executions/{id}/ci-cleanup", workers.authorize(func(w http.ResponseWriter, r *http.Request, grant *workerGrant) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			Worker   string `json:"worker"`
			File     string `json:"file"`
			Revision int64  `json:"revision"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Worker == "" || len(input.File) > 96 || input.File == "" || input.Revision < 1 {
			problem(w, 400, "invalid local file receipt")
			return
		}
		if grant.id != "" && grant.id != input.Worker {
			problem(w, 403, "cleanup belongs to another Worker")
			return
		}
		err := data.ConfirmLocalCIRemoval(r.Context(), input.Worker, grant.projects, id, input.File, input.Revision)
		respond(w, 204, nil, err)
	}))
	mux.HandleFunc("GET /projects/{id}/retention-history", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var before int64
		if value := r.URL.Query().Get("before"); value != "" {
			var err error
			before, err = strconv.ParseInt(value, 10, 64)
			if err != nil || before <= 0 {
				problem(w, 400, "before must be a positive revision ID")
				return
			}
		}
		history, err := data.RetentionHistory(r.Context(), id, before)
		respond(w, 200, history, err)
	}))
	const path = "/tasks/{task}/executions/{id}/ci/{run}/{file}"
	const probeRoute = "HEAD /internal" + path
	serve := func(w http.ResponseWriter, r *http.Request) {
		task, err := strconv.ParseInt(r.PathValue("task"), 10, 64)
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err != nil || task <= 0 {
			problem(w, 400, "invalid task ID")
			return
		}
		run, file := r.PathValue("run"), r.PathValue("file")
		key := fmt.Sprintf("ci/%d/%d/%s/%s", task, id, run, file)
		var content []byte
		var fileStatus int
		probe := r.Pattern == probeRoute
		err = data.AccessCIFile(r.Context(), task, id, run, file, func(want validation.FileDigest) error {
			if probe {
				return nil
			}
			if r.Method == http.MethodPut {
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, want.Bytes))
				if err != nil || validation.DigestBytes(body) != want {
					fileStatus = 422
					return errors.New("CI file does not match the stored receipt")
				}
				if err = saveArtifact(key, body); err != nil {
					fileStatus = 503
					return errors.New("CI artifact storage unavailable")
				}
				return nil
			}
			path, err := artifactPath(key)
			if err != nil {
				fileStatus = 503
				return errors.New("CI artifact storage unavailable")
			}
			f, err := os.Open(path)
			if err != nil {
				fileStatus = 503
				return errors.New("CI 文件尚未归档或存储不可用")
			}
			defer f.Close()
			content, err = io.ReadAll(io.LimitReader(f, want.Bytes+1))
			if err != nil || validation.DigestBytes(content) != want {
				fileStatus = 503
				return errors.New("CI 文件未通过完整性校验")
			}
			return nil
		})
		switch {
		case fileStatus != 0:
			problem(w, fileStatus, err.Error())
			return
		case err != nil:
			respond(w, 0, nil, err)
			return
		case r.Method == http.MethodPut || probe:
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, file))
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.Write(content)
	}
	mux.HandleFunc("GET "+path, authorized(owner, serve))
	workers.handle(mux, "PUT /internal"+path, workers.execution(data, serve))
	workers.handle(mux, probeRoute, workers.execution(data, serve))
}

func removeCIFile(f store.CIArchiveFile) error {
	if !validation.ValidRunID(f.Run) && !store.IsStartupLog(f.Execution, f.Run, f.File) {
		return errors.New("invalid archived CI run ID")
	}
	return removeArtifact(fmt.Sprintf("ci/%d/%d/%s/%s", f.Task, f.Execution, f.Run, f.File))
}

func sweepArtifacts(ctx context.Context, data *store.Store) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := data.PurgeProcessArchiveChunks(ctx); err != nil && ctx.Err() == nil {
			log.Printf("process archive cleanup: %v", err)
		}
		if err := data.ExpireLocalCIFiles(ctx); err != nil && ctx.Err() == nil {
			log.Printf("local CI retention: %v", err)
		}
		if err := data.PurgeCIFiles(ctx, removeCIFile); err != nil && ctx.Err() == nil {
			log.Printf("CI archive cleanup: %v", err)
		}
		if err := data.PurgeFeedbackArtifacts(ctx, removeArtifact); err != nil && ctx.Err() == nil {
			log.Printf("feedback attachment cleanup: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
