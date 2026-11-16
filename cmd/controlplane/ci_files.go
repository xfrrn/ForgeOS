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

func registerCIFiles(mux *http.ServeMux, data *store.Store, owner string, workers workerAccess) {
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
		current, err := data.ProjectCIRetention(r.Context(), id, replacement)
		respond(w, 200, current, err)
	})
	mux.HandleFunc("GET /projects/{id}/ci-retention", policy)
	mux.HandleFunc("PUT /projects/{id}/ci-retention", policy)
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
		case errors.Is(err, store.ErrCIExpired):
			problem(w, http.StatusGone, "CI 文件已按项目保留策略到期")
			return
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
	mux.HandleFunc("PUT /internal"+path, workers.execution(data, serve))
	mux.HandleFunc(probeRoute, workers.execution(data, serve))
}

func removeCIFile(f store.CIArchiveFile) error {
	if !validation.ValidRunID(f.Run) {
		return errors.New("invalid archived CI run ID")
	}
	return removeArtifact(fmt.Sprintf("ci/%d/%d/%s/%s", f.Task, f.Execution, f.Run, f.File))
}

func sweepCIFiles(ctx context.Context, data *store.Store) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := data.PurgeCIFiles(ctx, removeCIFile); err != nil && ctx.Err() == nil {
			log.Printf("CI archive cleanup: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
