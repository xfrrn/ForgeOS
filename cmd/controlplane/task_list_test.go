package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestTaskListPagesAndFilters(t *testing.T) {
	ctx := context.Background()
	data := store.New(testdb.Open(t))
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	api := routes(data, t.TempDir(), "owner", "worker", deliveryPolicy{})
	get := func(path string) store.TaskPage {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer owner")
		resp := httptest.NewRecorder()
		api.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, resp.Code, resp.Body.String())
		}
		var page store.TaskPage
		if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	if page := get("/tasks"); page.Items == nil || len(page.Items) != 0 || len(page.Counts) != 7 || page.Counts["ALL"] != 0 {
		t.Fatal("empty list:", page)
	}
	project, err := data.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	head, err := data.CreateTask(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.Claim(ctx, "local-worker", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	issue := store.IssueSpec{Issue: store.Issue{ID: 30, Number: 8, RepositoryID: 41,
		Repository: "Acme/Engine", Title: "Verify 10% change"}, BaseCommit: strings.Repeat("b", 40), Validation: "docs"}
	task, err := data.ImportIssue(ctx, project.ID, issue)
	if err != nil {
		t.Fatal(err)
	}
	page := get("/tasks?limit=1")
	if len(page.Items) != 1 || page.Items[0].ID != task.ID || page.NextBefore != task.ID ||
		page.Items[0].IssueNumber != 8 || page.Items[0].Repository != "Acme/Engine" || page.Items[0].Title != issue.Issue.Title {
		t.Fatal("first page:", page)
	}
	// A task arriving between pages must not shift the next page's position.
	if _, err := data.CreateTask(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
	page = get("/tasks?before=" + strconv.FormatInt(page.NextBefore, 10) + "&limit=1")
	if len(page.Items) != 1 || page.Items[0].ID != head.ID || page.Items[0].Worker != "local-worker" ||
		page.NextBefore != 0 || page.Counts["ALL"] != 3 || page.Counts["RUNNING"] != 1 || page.Counts["READY"] != 2 {
		t.Fatal("second page:", page)
	}
	id := strconv.FormatInt(task.ID, 10)
	for _, path := range []string{"/tasks?status=READY&q=ACME", "/tasks?q=%23" + id, "/tasks?q=T-" + id, "/tasks?q=%25"} {
		page = get(path)
		if len(page.Items) != 1 || page.Items[0].ID != task.ID || page.NextBefore != 0 || page.Counts["ALL"] != 3 {
			t.Fatal(path, page)
		}
	}
	page = get("/tasks?status=RUNNING&q=ACME")
	if len(page.Items) != 0 || page.Counts["RUNNING"] != 1 {
		t.Fatal("status and search must both match:", page)
	}
	page = get("/tasks?status=RUNNING&q=git%20head")
	if len(page.Items) != 1 || page.Items[0].ID != head.ID {
		t.Fatal("local task title search:", page)
	}
}
