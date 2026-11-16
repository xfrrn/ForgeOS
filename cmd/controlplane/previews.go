package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

var previewHost = regexp.MustCompile(`^p-([0-9a-f]{48})\.localhost$`)

func publicPath(name string) bool {
	if name == "." || !fs.ValidPath(name) || len(name) > 1024 || strings.ContainsAny(name, "\\:\x00\r\n\t") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

func previewBlob(ctx context.Context, repo, commit, name string) ([]byte, error) {
	if !publicPath(name) || !gitCommit.MatchString(commit) {
		return nil, store.ErrNotFound
	}
	row, overflow, err := localrepo.LimitedGit(ctx, repo, 2048, "--literal-pathspecs", "ls-tree", "-z", commit, "--", name)
	header, foundName, ok := strings.Cut(strings.TrimSuffix(string(row), "\x00"), "\t")
	fields := strings.Fields(header)
	if err != nil || overflow || !ok || foundName != name || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !gitCommit.MatchString(fields[2]) {
		return nil, store.ErrNotFound
	}
	data, overflow, err := localrepo.LimitedGit(ctx, repo, 8<<20, "cat-file", "blob", fields[2])
	if overflow {
		return nil, errors.New("preview asset exceeds 8 MiB")
	}
	return data, err
}

// The preview origin serves only committed assets. It has no control-plane routes or credentials.
func previewAssets(data *store.Store, root string, port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; worker-src 'none'; frame-ancestors 'none'; object-src 'none'; form-action 'self'; base-uri 'self'")
		h, p, err := net.SplitHostPort(r.Host)
		match := previewHost.FindStringSubmatch(h)
		if err != nil || p != strconv.Itoa(port) || match == nil || (r.Method != "GET" && r.Method != "HEAD") || r.Header.Get("Service-Worker") != "" {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || strings.HasSuffix(name, "/") {
			name += "index.html"
		}
		if !publicPath(name) {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		registered, commit, dir, err := data.PreviewSource(ctx, match[1], port)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		repo, err := localrepo.ResolveRepo(ctx, root, registered)
		if err != nil {
			http.Error(w, "preview repository unavailable", 503)
			return
		}
		body, err := previewBlob(ctx, repo, commit, dir+"/"+name)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "preview asset unavailable", 422)
			return
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-ForgeOS-Commit", commit)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
	}
}

func startPreviews(mux *http.ServeMux, data *store.Store, root, owner, addr string) (func(), error) {
	var port int
	done := func() {}
	if addr != "" {
		h, p, err := net.SplitHostPort(addr)
		var errPort error
		port, errPort = strconv.Atoi(p)
		if err != nil || h != "127.0.0.1" || errPort != nil || port < 1 || port > 65535 {
			return nil, errors.New("FORGEOS_PREVIEW_ADDR must be 127.0.0.1 and a fixed port")
		}
		ln, err := net.Listen("tcp4", addr)
		if err != nil {
			return nil, err
		}
		server := &http.Server{Handler: previewAssets(data, root, port), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
		go func() {
			if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("preview listener stopped: %v", err)
			}
		}()
		done = func() { server.Close() }
	}
	mux.HandleFunc("GET /tasks/{id}/previews", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		execution, err := strconv.ParseInt(r.URL.Query().Get("execution_id"), 10, 64)
		if err != nil || execution <= 0 {
			problem(w, 400, "provide execution_id")
			return
		}
		receipts, err := data.Previews(r.Context(), id, execution)
		respond(w, 200, map[string]any{"enabled": port != 0, "previews": receipts}, err)
	}))
	mux.HandleFunc("POST /tasks/{id}/previews", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if port == 0 {
			problem(w, 503, "configure FORGEOS_PREVIEW_ADDR")
			return
		}
		var req struct {
			Execution int64  `json:"execution_id"`
			Commit    string `json:"commit"`
			Directory string `json:"directory"`
			Seconds   int    `json:"ttl_seconds"`
		}
		if !decodeLimited(w, r, &req, 8<<10) {
			return
		}
		if req.Seconds == 0 {
			req.Seconds = 3600
		}
		if !publicPath(req.Directory) || !gitCommit.MatchString(req.Commit) || req.Execution < 1 || req.Seconds < 1 || req.Seconds > 86400 {
			problem(w, 400, "provide a public subdirectory, fixed execution and commit, and TTL from 1 to 86400 seconds")
			return
		}
		task, err := data.Task(r.Context(), id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		var candidate bool
		for _, a := range task.Attempts {
			if a.ID == req.Execution && a.Result != nil && a.Result.Commit == req.Commit && a.Result.Validation != nil && a.Result.Validation.Commit == req.Commit && a.Result.Validation.ExitCode == 0 {
				candidate = true
				break
			}
		}
		if !candidate || task.Spec == nil {
			problem(w, 409, "execution has no matching validated candidate")
			return
		}
		project, err := data.Project(r.Context(), task.ProjectID)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(r.Context(), root, project.Path)
		if err != nil || project.RepositoryID != task.Spec.Issue.RepositoryID {
			problem(w, 409, "candidate repository unavailable")
			return
		}
		if _, err = previewBlob(r.Context(), repo, req.Commit, req.Directory+"/index.html"); err != nil {
			problem(w, 422, "directory must contain a committed, readable index.html up to 8 MiB")
			return
		}
		receipt, err := data.OpenPreview(r.Context(), id, req.Execution, req.Commit, req.Directory, port, req.Seconds)
		respond(w, 200, receipt, err)
	}))
	mux.HandleFunc("DELETE /tasks/{id}/previews/{preview}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		err := data.StopPreview(r.Context(), id, r.PathValue("preview"))
		respond(w, 204, nil, err)
	}))
	return done, nil
}
