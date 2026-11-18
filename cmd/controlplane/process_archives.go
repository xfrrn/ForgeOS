package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/store"
)

func registerProcessArchives(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	const path = "/tasks/{task}/executions/{id}/processes/{instance}/archive"
	serve := serveProcessArchive(data, "", 0)
	mux.HandleFunc("GET "+path, authorizedFor(owner, 30*time.Minute, serve))
	workers.handle(mux, "HEAD /internal"+path, workers.execution(data, serve))
	workers.handle(mux, "PUT /internal"+path, workers.executionFor(data, 5*time.Minute, serve))
	mux.HandleFunc("GET /role-processes/{id}/instances/{instance}/archive", authorizedFor(owner, 30*time.Minute, serve))
}

func registerRoleArchiveUploads(mux *http.ServeMux, data *store.Store, token, role string, project int64) {
	path := "/internal/" + role + "/role-processes/{id}/instances/{instance}/archive"
	serve := serveProcessArchive(data, role, project)
	mux.HandleFunc("HEAD "+path, authorized(token, serve))
	mux.HandleFunc("PUT "+path, authorizedFor(token, 5*time.Minute, serve))
}

func serveProcessArchive(data *store.Store, role string, project int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		instance, err := strconv.ParseInt(r.PathValue("instance"), 10, 64)
		if err != nil || instance <= 0 {
			problem(w, 400, "invalid process instance")
			return
		}
		ref := store.ArchiveTarget{Instance: instance}
		name := "role-process"
		if raw := r.PathValue("task"); raw != "" {
			task, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || task <= 0 {
				problem(w, 400, "invalid task")
				return
			}
			ref.Task, ref.Execution = task, id
			name = "execution"
		} else {
			ref.RoleProcess, ref.Project, ref.Role = id, project, role
		}
		ctx := r.Context()
		switch r.Method {
		case http.MethodHead:
			if !strings.HasPrefix(r.Pattern, "HEAD /internal/") {
				problem(w, http.StatusMethodNotAllowed, "use GET to download the archive")
				return
			}
			var status store.ArchiveTransfer
			status, err = data.ArchiveUploadStatus(ctx, ref)
			if err == nil {
				w.Header().Set("ForgeOS-Archive-Offset", strconv.FormatInt(status.Received, 10))
				w.WriteHeader(204)
				return
			}
		case http.MethodPut:
			_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(time.Minute))
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute))
			offset, parseErr := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
			if parseErr != nil || offset < 0 {
				problem(w, 400, "invalid archive byte offset")
				return
			}
			block, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, store.ProcessArchiveChunkSize))
			if readErr != nil {
				problem(w, 400, "invalid process archive block")
				return
			}
			err = data.PutArchiveChunk(ctx, ref, offset, block)
			if err == nil {
				w.WriteHeader(204)
				return
			}
		default:
			wrote := false
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))
			err = data.DownloadProcessArchive(ctx, ref, func(a store.ProcessArchive) io.Writer {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%d-process-%d.jsonl"`, name, id, instance))
				w.Header().Set("Content-Length", strconv.FormatInt(a.Bytes, 10))
				w.WriteHeader(200)
				wrote = true
				return w
			})
			if wrote {
				if err != nil && ctx.Err() == nil {
					log.Printf("process archive download for %s %d instance %d: %v", name, id, instance, err)
				}
				return
			}
		}
		switch {
		case r.Method == http.MethodGet && errors.Is(err, store.ErrConflict):
			problem(w, 422, "进程日志未通过完整性校验")
		case errors.Is(err, store.ErrProcessArchiveExpired):
			problem(w, 410, "进程归档已到期")
		case errors.Is(err, store.ErrProcessArchivePending):
			problem(w, 409, "节点尚未完成归档传输")
		default:
			respond(w, 200, nil, err)
		}
	}
}
