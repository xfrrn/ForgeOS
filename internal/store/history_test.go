package store

import (
	"context"
	"strings"
	"testing"
)

func TestHistoryRanksOnlyProjectSourcesAndRetainsRevisionStatus(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		p, err := s.CreateProject(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		issue := Issue{ID: p.ID, Number: 1, RepositoryID: p.ID, Body: name + " 私有项目历史", Title: "shared query", URL: "https://github.com/example/" + name + "/issues/1", UpdatedAt: "2026-01-01T10:00:00Z"}
		task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: issue, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
		if err != nil {
			t.Fatal(err)
		}
		issue.Body, issue.UpdatedAt = "updated body", "2026-01-01T11:00:00Z"
		if _, err = s.ObserveIssue(ctx, p.ID, issue); err != nil {
			t.Fatal(err)
		}
		hits, err := s.SearchHistory(ctx, p.ID, 0, "requirement", "项目历史")
		if err != nil || len(hits) != 1 || hits[0].Status != "historical" || hits[0].TaskID != task.ID || hits[0].Version != 1 || hits[0].URL != issue.URL {
			t.Fatalf("historical result: %v %v", hits, err)
		}
		hits, err = s.SearchHistory(ctx, p.ID, 0, "", "shared query")
		if err != nil || len(hits) != 2 {
			t.Fatalf("shared query: %v %v", hits, err)
		}
		for _, hit := range hits {
			if hit.SourceID != issue.ID || hit.TaskID != task.ID || hit.ProjectID != p.ID {
				t.Fatalf("wrong source: %v", hit)
			}
		}
	}
}
