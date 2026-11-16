package localrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInventoryPinsCommitAndIgnoresPrivateAndVendoredFiles(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatal(err, string(out))
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet")
	files := map[string]string{"README.md": "example", "go.mod": "module example.org/example\n\nrequire example.org/library v1.0.0\n",
		"web/package.json":     `{"scripts":{"dev":"private-body"},"dependencies":{"@example/pkg":"private-version"}}`,
		"invalid/package.json": "~", "vendor/pkg/package.json": "{}", ".env": "private-values"}
	for name, body := range files {
		p := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "-f", ".")
	blob := git("hash-object", "README.md")
	git("update-index", "--add", "--cacheinfo", "120000,"+blob+",linked/package.json")
	git("-c", "user.name=QA", "-c", "user.email=qa@example.org", "commit", "--quiet", "-m", "sources")
	commit := git("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "web", "package.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Inventory(context.Background(), repo, commit)
	if err != nil || len(got) != 4 {
		t.Fatal(got, err)
	}
	if got[0].Blob != blob || got[0].Kind != "guide" || !reflect.DeepEqual(got[1].Dependencies, []string{"example.org/library"}) ||
		got[2].Problem != "invalid_manifest" || !reflect.DeepEqual(got[3].Dependencies, []string{"@example/pkg"}) || !reflect.DeepEqual(got[3].Scripts, []string{"dev"}) {
		t.Fatal(got)
	}
	if _, err := Inventory(context.Background(), repo, blob); err == nil {
		t.Fatal("accepted a blob as the checked revision")
	}
}
