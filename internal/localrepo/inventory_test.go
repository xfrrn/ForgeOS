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

func TestComposerInventoryUsesOnlyDeclaredPackageAndScriptNames(t *testing.T) {
	file := InventoryFile{Kind: inventoryKind("app/composer.json")}
	file.readManifest([]byte(`{"require":{"php":"^8","composer-runtime-api":"^2","example/project--name":"dev-main","invalid/url/extra":"1"},
		"require-dev":{"ext-pdo":"*","example/project--name":"dev-main"},"suggest":{"vendor/optional":"suggestion"},"scripts":{"test":["handler","command"],"start":"command","unknown":[null],"--option":"command"}}`))
	if file.Kind != "composer" || file.Problem != "" || !reflect.DeepEqual(file.Dependencies, []string{"composer-runtime-api", "example/project--name", "ext-pdo", "php"}) || !reflect.DeepEqual(file.Scripts, []string{"start", "test"}) {
		t.Fatal(file)
	}
	for _, bad := range []string{"null", `{"require":[]}`, `{"scripts":"run"}`} {
		file = InventoryFile{Kind: "composer"}
		file.readManifest([]byte(bad))
		if file.Problem != "invalid_manifest" || len(file.Dependencies)+len(file.Scripts) != 0 {
			t.Fatal(file)
		}
	}
}
