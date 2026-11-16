package main

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"forgeos/internal/store"
)

var webAsset = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*\.(js|css|woff2?|png|webp|svg|ico)$`)

func registerWebRoutes(mux *http.ServeMux, data *store.Store, repoRoot, owner string) {
	root := os.Getenv("FORGEOS_WEB_ROOT")
	if root == "" {
		root = filepath.Join("web", "dist")
	}
	page := func(w http.ResponseWriter, r *http.Request) { serveWebFile(w, r, root, "index.html") }
	mux.HandleFunc("GET /{$}", page)
	mux.HandleFunc("GET /ui/{$}", page)
	mux.HandleFunc("GET /ui/assets/{asset}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("asset")
		if !webAsset.MatchString(name) || !filepath.IsLocal(name) {
			http.NotFound(w, r)
			return
		}
		serveWebFile(w, r, root, filepath.Join("assets", name))
	})
	mux.HandleFunc("GET /tasks/{id}/diff", authorized(owner, taskDiff(data, repoRoot)))
}

func serveWebFile(w http.ResponseWriter, r *http.Request, root, name string) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; media-src 'self' blob:; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	file, err := openWebFile(root, name)
	if err != nil {
		if name == "index.html" {
			http.Error(w, "任务工作台尚未构建。请在项目根目录运行 npm --prefix web ci 和 npm --prefix web run build。", http.StatusServiceUnavailable)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	switch filepath.Ext(name) {
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	http.ServeContent(w, r, name, stat.ModTime(), file)
}

func openWebFile(root, name string) (*os.File, error) {
	// A build directory may be symlinked, but its files must resolve inside it.
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(base, name))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, os.ErrPermission
	}
	return os.Open(path)
}
