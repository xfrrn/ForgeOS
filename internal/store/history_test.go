package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHistoryRanksOnlyProjectSourcesAndRetainsRevisionStatus(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var alphaProject, alphaTask int64
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
		if name == "alpha" {
			alphaProject, alphaTask = p.ID, task.ID
		}
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
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data struct{ Input []string }
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil || len(data.Input) != 1 ||
			(data.Input[0] != "shared query alpha 私有项目历史" && data.Input[0] != "shared query updated body") {
			t.Error("history outside the selected scope", data.Input, err)
		}
		calls.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0]}]}`))
	}))
	defer server.Close()
	s.embeddings = &materialEncoder{endpoint: server.URL, model: "fixture", client: server.Client()}
	for _, ref := range []string{"", strings.Repeat("a", 40), "https://github.com/example/beta/issues/1"} {
		if _, err := s.SearchHistory(ctx, alphaProject, alphaTask, "requirement", ref); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := s.SearchHistory(ctx, alphaProject, alphaTask, "requirement", "https://github.com/example/alpha/issues/1")
	if err != nil || len(hits) != 2 || hits[0].Status != "latest" || hits[1].Status != "historical" {
		t.Fatal("source URL did not resolve both revisions", hits, err)
	}
	if err := s.indexHistory(ctx, alphaProject, alphaTask, "comment", 2); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("browsing, exact references or an empty source contacted the encoder")
	}
	for range 2 {
		if err := s.indexHistory(ctx, alphaProject, alphaTask, "requirement", 2); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("two eligible versions should be encoded once each, got %d requests", calls.Load())
	}
}
