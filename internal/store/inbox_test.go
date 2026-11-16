package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInboxSnapshotAndConversion(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "inbox-repo")
	if err != nil {
		t.Fatal(err)
	}
	issue := Discussion{ID: 11, Number: 2, Title: "Discussion"}
	if err := s.ReplaceInbox(ctx, p.ID, 80, []Discussion{issue, issue}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceInbox(ctx, p.ID, 81, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("repository mismatch: %v", err)
	}
	items, err := s.Inbox(ctx, p.ID, 0)
	if err != nil || len(items) != 1 || items[0] != issue {
		t.Fatalf("inbox after failed scan: %v %v", items, err)
	}
	spec := IssueSpec{Issue: Issue{ID: issue.ID, Number: issue.Number, RepositoryID: 80},
		BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	if _, err := s.ImportIssue(ctx, p.ID, spec); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceInbox(ctx, p.ID, 80, []Discussion{issue}); err != nil {
		t.Fatal(err)
	}
	if items, err := s.Inbox(ctx, p.ID, 0); err != nil || len(items) != 0 {
		t.Fatalf("converted discussion reappeared: %v %v", items, err)
	}
}
