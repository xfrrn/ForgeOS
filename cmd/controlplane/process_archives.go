package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"forgeos/internal/store"
)

func registerProcessArchives(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	const path = "/tasks/{task}/executions/{id}/processes/{instance}/archive"
	serve := func(w http.ResponseWriter, r *http.Request) {
		execution, ok := pathID(w, r)
		if !ok {
			return
		}
		task, e1 := strconv.ParseInt(r.PathValue("task"), 10, 64)
		instance, e2 := strconv.ParseInt(r.PathValue("instance"), 10, 64)
		if e1 != nil || task <= 0 || e2 != nil || instance <= 0 {
			problem(w, 400, "invalid task or process instance")
			return
		}
		ctx := r.Context()
		var err error
		switch r.Method {
		case http.MethodHead:
			if r.Pattern != "HEAD /internal"+path {
				problem(w, http.StatusMethodNotAllowed, "use GET to download the archive")
				return
			}
			var status store.ArchiveTransfer
			status, err = data.ArchiveUploadStatus(ctx, task, execution, instance)
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
			err = data.PutArchiveChunk(ctx, task, execution, instance, offset, block)
			if err == nil {
				w.WriteHeader(204)
				return
			}
		default:
			wrote := false
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))
			err = data.DownloadProcessArchive(ctx, task, execution, instance, func(a store.ProcessArchive) io.Writer {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="execution-%d-process-%d.jsonl"`, execution, instance))
				w.Header().Set("Content-Length", strconv.FormatInt(a.Bytes, 10))
				w.WriteHeader(200)
				wrote = true
				return w
			})
			if wrote {
				if err != nil && ctx.Err() == nil {
					log.Printf("process archive download for execution %d instance %d: %v", execution, instance, err)
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
	mux.HandleFunc("GET "+path, authorizedFor(owner, 30*time.Minute, serve))
	workers.handle(mux, "HEAD /internal"+path, workers.execution(data, serve))
	workers.handle(mux, "PUT /internal"+path, workers.executionFor(data, 5*time.Minute, serve))
}
