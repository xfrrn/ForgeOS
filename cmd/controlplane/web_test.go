package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebEntryAndAssets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FORGEOS_WEB_ROOT", root)
	api := routes(nil, t.TempDir(), "owner", legacyWorker("worker"), deliveryPolicy{})
	get := func(url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		return w
	}
	if w := get("/"); w.Code != 503 || !strings.Contains(w.Body.String(), "npm") {
		t.Fatal("missing build did not provide setup instructions")
	}
	if w := get("/tasks/1"); w.Code != 401 {
		t.Fatal("missing frontend changed API authentication")
	}
	if err := os.Mkdir(filepath.Join(root, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{"index.html": "workbench", "assets/index-1.js": "console.log(1)", "operator.txt": "private"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, url := range []string{"/", "/ui/", "/ui/assets/index-1.js"} {
		w := get(url)
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") ||
			w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("unexpected %s response: %d %v", url, w.Code, w.Header())
		}
	}
	for _, url := range []string{"/ui/assets/", "/ui/operator.txt", "/ui/assets/..%2Foperator.txt", "/ui/assets/..%5Coutside.js"} {
		if w := get(url); w.Code != 404 {
			t.Errorf("%s exposed unexpected resource: %d", url, w.Code)
		}
	}
	t.Run("outside symlink", func(t *testing.T) {
		external := filepath.Join(t.TempDir(), "outside.js")
		if err := os.WriteFile(external, []byte("external"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(root, "assets", "outside.js")); err != nil {
			t.Skip("filesystem does not permit a symlink")
		}
		if w := get("/ui/assets/outside.js"); w.Code != 404 {
			t.Fatal("asset escaped build directory")
		}
	})
}
