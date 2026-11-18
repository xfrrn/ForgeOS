package localrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDocumentSearchPinsBlobsAndMatchesAcrossParts(t *testing.T) {
	repo := t.TempDir()
	ctx := context.Background()
	git := func(args ...string) string {
		t.Helper()
		raw, err := Git(ctx, repo, args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return strings.TrimSpace(string(raw))
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init")
	git("config", "user.name", "ForgeOS QA")
	git("config", "user.email", "qa@example.invalid")
	git("config", "core.autocrlf", "false")
	prefix := strings.Repeat("x", (64<<10)-1)
	body := prefix + "K界测量" + strings.Repeat("乙", 100)
	write("guide.md", body)
	write("private.env", body)
	write("binary.md", body+"\x00")
	git("add", ".")
	blob := git("rev-parse", ":guide.md")
	git("update-index", "--add", "--cacheinfo", "120000,"+blob+",link.md")
	git("commit", "-qm", "original")
	base := git("rev-parse", "HEAD")
	write("guide.md", "replacement")
	git("add", "guide.md")
	git("commit", "-qm", "new version")
	write("guide.md", body)
	result, err := SearchDocuments(ctx, repo, base, "k界测量", nil)
	if err != nil || len(result.Items) != 1 || result.Commit != base || result.CommittedAt == "" {
		t.Fatal(result, err)
	}
	h := result.Items[0]
	if h.Path != "guide.md" || h.Blob != blob || h.Start != int64(len(prefix)) || body[h.Start:h.End] != "K界测量" ||
		!utf8.ValidString(h.Excerpt) || body[h.ExcerptStart:h.ExcerptStart+int64(len(h.Excerpt))] != h.Excerpt {
		t.Fatal(h)
	}
	now, err := SearchDocuments(ctx, repo, "", "k界测量", nil)
	if err != nil || len(now.Items) != 0 || now.Commit == base {
		t.Fatal(now, err)
	}
	if _, err := SearchDocuments(ctx, repo, blob, "x", nil); err == nil {
		t.Fatal("accepted a blob as a commit")
	}
	var visited []DocumentMatch
	_, err = SearchDocuments(ctx, repo, base, "absent", func(h DocumentMatch) error {
		visited = append(visited, h)
		return nil
	})
	if err != nil || len(visited) != 1 || visited[0].Path != "guide.md" || visited[0].Blob != blob || visited[0].Excerpt != "" {
		t.Fatal("only a fully validated document may be ranked", visited, err)
	}
}
