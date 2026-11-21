package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentChangesBindCommittedBlobs(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=QA", "-c", "user.email=qa@example.invalid", "-C", repo}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(string(output), err)
		}
	}
	write := func(name, body string) {
		t.Helper()
		f := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(f), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.CreateProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	observe := func() {
		t.Helper()
		errors := make(chan error, 2)
		for range 2 {
			go func() { errors <- s.ObserveDocumentHead(ctx, p.ID, repo) }()
		}
		for range 2 {
			if err := <-errors; err != nil {
				t.Fatal(err)
			}
		}
	}
	git("init", "--quiet")
	git("config", "core.autocrlf", "false")
	write("README.md", "Initial version")
	git("add", ".")
	git("commit", "-qm", "Baseline")
	observe()
	if j, err := s.ClaimCuration(ctx, p.ID, false, false); err != nil || j != nil {
		t.Fatal(j, err)
	}
	write("docs/recovery.md", "Observed offset 418.")
	write("docs/binary.md", "bad\x00file")
	write("docs/large.md", strings.Repeat("a", 256<<10+1)+"\x00")
	write("vendor/notes.md", "unrelated")
	git("add", ".")
	git("commit", "-qm", "Add repository guides")
	write("docs/recovery.md", "uncommitted draft")
	observe()
	if job, err := s.ClaimCuration(ctx, p.ID, false, false); err != nil || job != nil {
		t.Fatal("repository text was offered to Curator", job, err)
	}
	var src struct {
		Revision int64
		Content  string
	}
	var curationID int64
	if err := s.db.QueryRowContext(ctx, `SELECT curation_id,revision,content FROM curation_sources WHERE project_id=$1 AND title='docs/recovery.md'`, p.ID).Scan(&curationID, &src.Revision, &src.Content); err != nil || src.Content != "Observed offset 418." {
		t.Fatal(src, err)
	}
	// Simulate an approved fact retained by an upgraded installation.
	if _, err := s.db.ExecContext(ctx, `UPDATE material_curations SET extraction='{"facts":[{"statement":"offset","quote":"418"}]}', review='{"level":"D1","verdict":"approve"}' WHERE id=$1`, curationID); err != nil {
		t.Fatal(err)
	}
	if hits, err := s.SearchExperience(ctx, p.ID, "offset"); err != nil || string(hits) != "[]" {
		t.Fatal("archived Git fact is searchable", string(hits), err)
	}
	if _, err := s.DocumentCuration(ctx, p.ID+1, src.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	list, err := s.DocumentChanges(ctx, p.ID, 0)
	if err != nil || !strings.Contains(string(list), "non_text") || strings.Contains(string(list), "size_limit") || strings.Contains(string(list), "vendor/") {
		t.Fatal(string(list), err)
	}
	git("rm", "-f", "docs/recovery.md")
	git("commit", "-qm", "Remove recovery guide")
	observe()
	var current bool
	if err := s.db.QueryRowContext(ctx, "SELECT is_current FROM curation_sources WHERE curation_id=$1", curationID).Scan(&current); err != nil || current {
		t.Fatal("deleted source is current", err)
	}
	history, err := s.DocumentCuration(ctx, p.ID, src.Revision)
	if err != nil || !strings.Contains(string(history), src.Content) {
		t.Fatal(string(history), err)
	}

	t.Run("multipart recovery and sparse source", func(t *testing.T) {
		body := strings.Repeat("重复行。\r\n", 25000) + "final fact"
		write("docs/large.md", body)
		write("docs/spaces.md", strings.Repeat(" ", (256<<10)+1))
		git("add", "docs/large.md", "docs/spaces.md")
		git("commit", "-qm", "Record long text")
		observe()
		var id int64
		var parts int
		if err := s.db.QueryRowContext(ctx, `SELECT id,part_count FROM repository_document_changes WHERE project_id=$1 AND path='docs/large.md' ORDER BY id DESC LIMIT 1`, p.ID).Scan(&id, &parts); err != nil || parts < 2 {
			t.Fatal(parts, err)
		}
		var joined strings.Builder
		for part := 1; part <= parts; part++ {
			r, err := s.DocumentPartCuration(ctx, p.ID, id, part)
			var doc struct {
				Content  string
				Document struct {
					Offset int64 `json:"byte_offset"`
				}
			}
			if err != nil || json.Unmarshal(r, &doc) != nil || doc.Document.Offset != int64(joined.Len()) {
				t.Fatal(part, err)
			}
			joined.WriteString(doc.Content)
		}
		if joined.String() != body {
			t.Fatal("multipart content changed")
		}
		if _, err := s.DocumentCuration(ctx, p.ID, id); !errors.Is(err, ErrNotFound) {
			t.Fatal("missing part selector", err)
		}
		var orphaned int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM repository_document_parts p JOIN repository_document_changes d ON d.id=p.change_id WHERE d.project_id=$1 AND d.problem<>'partitioned'`, p.ID).Scan(&orphaned); err != nil || orphaned != 0 {
			t.Fatal("partial source left behind", orphaned, err)
		}
	})
	t.Run("resume history between observations", func(t *testing.T) {
		write("docs/offline.md", "Interim observation")
		git("add", "docs/offline.md")
		git("commit", "-qm", "Record intermediate evidence")
		for range 32 {
			git("commit", "-q", "--allow-empty", "-m", "Advance offline branch")
		}
		git("rm", "docs/offline.md")
		git("commit", "-qm", "Retire intermediate evidence")
		more, err := s.observeDocumentBatch(ctx, p.ID, repo)
		if err != nil || !more {
			t.Fatal("expected a durable partial scan", more, err)
		}
		var count int
		countChanges := func() int {
			t.Helper()
			if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM repository_document_changes WHERE project_id=$1 AND path='docs/offline.md'`, p.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			return count
		}
		if countChanges() != 1 {
			t.Fatal("missing intermediate commit", count)
		}
		if err := New(s.db).ObserveDocumentHead(ctx, p.ID, repo); err != nil {
			t.Fatal(err)
		}
		observe()
		if countChanges() != 2 {
			t.Fatal("missed final removal or repeated an earlier commit", count)
		}
		if err := s.db.QueryRowContext(ctx, `SELECT is_current FROM curation_sources WHERE project_id=$1 AND title='docs/offline.md'`, p.ID).Scan(&current); err != nil || current {
			t.Fatal("superseded intermediate source", err)
		}
	})
}
