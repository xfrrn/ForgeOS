package localrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitReadsOriginalObjectAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := Git(context.Background(), dir, args...)
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	writeBlob := func(contents string) string {
		t.Helper()
		path := filepath.Join(dir, "README.md")
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		return git("hash-object", "-w", path)
	}
	approved, replacement := writeBlob("approved"), writeBlob("replacement")
	git("replace", approved, replacement)
	if got := git("cat-file", "blob", approved); got != "approved" {
		t.Fatalf("content for frozen SHA changed to %q", got)
	}
}
