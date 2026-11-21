package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func TestRepositoryDocumentBrowser(t *testing.T) {
	root, repo, base := issueTestRepo(t)
	git := func(args ...string) string { return deliveryGit(t, repo, args...) }
	git("config", "core.autocrlf", "false")
	git("config", "user.name", "ForgeOS QA")
	git("config", "user.email", "qa@example.invalid")
	write := func(path, content string) {
		t.Helper()
		target := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const name = "docs/设计 + [一期].md"
	const original = "# 工程指南\r\n\r\n原始版本\n"
	write(name, original)
	write("docs/空白.txt", " \n")
	write("docs/large.md", strings.Repeat("x", (8<<20)+1))
	write("binary.md", "text\x00")
	write("private.env", "private")
	write("vendor/private.md", "private")
	git("add", ".")
	blob := git("rev-parse", ":"+name)
	git("update-index", "--add", "--cacheinfo", "120000,"+blob+",link.md")
	git("commit", "-qm", "documents")
	v1 := git("rev-parse", "HEAD")
	write(name, "# 新版\n")
	git("add", "--", name)
	git("commit", "-qm", "new version")
	v2 := git("rev-parse", "HEAD")
	write(name, "未提交编辑")
	write("draft.md", "未提交资料")
	// Create an object in this same repository that is outside HEAD's ancestry.
	unrelated := git("commit-tree", v1+"^{tree}", "-m", "other root")

	mux := http.NewServeMux()
	workers := legacyWorker("worker")
	workers.grants[0].projects = []int64{1}
	registerDocumentBrowser(mux, &diffFixture{project: store.Project{ID: 1, Path: repo}}, root, "owner", workers)
	get := func(path, token string, status int, dest any) {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)
		if resp.Code != status {
			t.Fatalf("%s: got %d, want %d: %s", path, resp.Code, status, resp.Body)
		}
		if dest != nil {
			if err := json.Unmarshal(resp.Body.Bytes(), dest); err != nil {
				t.Fatal(err)
			}
		}
	}
	const docs = "/projects/1/documents/"
	var catalog struct {
		Commit, Branch string
		Documents      []localrepo.TrackedBlob
	}
	get(docs+"tree", "owner", 200, &catalog)
	var paths []string
	for _, entry := range catalog.Documents {
		paths = append(paths, entry.Path)
		if !gitCommit.MatchString(entry.Blob) {
			t.Fatalf("missing blob on %s", entry.Path)
		}
	}
	if catalog.Commit != v2 || catalog.Branch != git("branch", "--show-current") ||
		!slices.Equal(paths, []string{"binary.md", "docs/large.md", "docs/空白.txt", name}) {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	get(docs+"tree?commit="+base, "owner", 200, &catalog)
	if catalog.Commit != base || catalog.Documents == nil || len(catalog.Documents) != 0 {
		t.Fatalf("expected an empty historical tree: %+v", catalog)
	}
	read := func(commit, path string, status int) (doc struct{ Commit, Path, Blob, Content string }) {
		t.Helper()
		get(docs+"read?commit="+commit+"&path="+url.QueryEscape(path), "owner", status, &doc)
		return doc
	}
	if doc := read(v1, name, 200); doc.Commit != v1 || doc.Path != name || doc.Blob != blob || doc.Content != original {
		t.Fatal("wrong fixed version", doc)
	}
	if doc := read(v2, name, 200); doc.Content != "# 新版\n" || doc.Blob == blob {
		t.Fatal("wrong current version", doc)
	}
	if doc := read(v2, "docs/空白.txt", 200); doc.Content != " \n" {
		t.Fatal("lost whitespace", doc)
	}
	for _, path := range []string{"draft.md", "link.md", "vendor/private.md", "private.env", "docs", "*.md"} {
		read(v2, path, 404)
	}
	read(v2, "../README.md", 400)
	read(v2, "binary.md", 415)
	read(v2, "docs/large.md", 413)
	read(unrelated, name, 422)
	get(docs+"tree?commit="+blob, "owner", 422, nil)
	get(docs+"tree?commit=HEAD", "owner", 400, nil)
	get(docs+"read?commit=&path=README.md", "owner", 400, nil)
	get(docs+"read?path=README.md", "owner", 404, nil)
	get(docs+"tree", "worker", 401, nil)
	get("/internal"+docs+"tree", "worker", 200, nil)
	get("/internal/projects/2/documents/tree", "worker", 403, nil)
	get("/projects/2/documents/tree", "owner", 404, nil)

	currentURL := docs + "read?path=" + url.QueryEscape(name)
	current := func(commit, body string) {
		t.Helper()
		var doc struct{ Commit, Path, Blob, Content string }
		get(currentURL, "owner", 200, &doc)
		if doc.Commit != commit || doc.Path != name || doc.Blob != git("rev-parse", commit+":"+name) || doc.Content != body {
			t.Fatalf("wrong document at current HEAD %s: %+v", commit, doc)
		}
	}
	get(currentURL, "worker", 401, nil)
	get("/internal"+currentURL, "worker", 200, nil)
	get(strings.Replace("/internal"+currentURL, "/projects/1/", "/projects/2/", 1), "worker", 403, nil)
	current(v2, "# 新版\n")
	git("--literal-pathspecs", "add", "--", name)
	current(v2, "# 新版\n")
	git("commit", "-qm", "publish edit")
	current(git("rev-parse", "HEAD"), "未提交编辑")
	git("switch", "-c", "release-docs", v1)
	current(v1, original)
	git("--literal-pathspecs", "rm", "--", name)
	git("commit", "-qm", "remove obsolete document")
	write(name, "本地草稿")
	get(currentURL, "owner", 404, nil)
	if doc := read(v1, name, 200); doc.Content != original {
		t.Fatal("lost the reachable historical document after deletion", doc)
	}
}
