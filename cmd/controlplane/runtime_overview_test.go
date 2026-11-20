package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/runtimeinfo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestRuntimeStatusAndWorkerDiagnostics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := testdb.Open(t)
	data := store.New(db)
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	handler := routes(data, root, "owner", legacyWorker("worker"), nil)
	registerRuntimeStatus(ctx, handler, db, root, "owner")
	read := func(path, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		reply := httptest.NewRecorder()
		handler.ServeHTTP(reply, req)
		if reply.Code != want {
			t.Fatalf("GET %s: status %d, want %d: %s", path, reply.Code, want, reply.Body)
		}
		return reply
	}
	read("/workers/control-plane", "worker", http.StatusUnauthorized)
	if reply := read("/workers/control-plane", "owner", http.StatusOK); !strings.Contains(reply.Body.String(), `"postgresql":"reachable"`) {
		t.Fatalf("unexpected control plane status: %s", reply.Body)
	}
	beat := func(sample string, want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/internal/workers/heartbeat?diagnostics="+url.QueryEscape(sample), strings.NewReader(
			`{"worker":"local-test","os":"windows","arch":"amd64","cpus":8,"slots":1,"capabilities":{}}`))
		req.Header.Set("Authorization", "Bearer worker")
		reply := httptest.NewRecorder()
		handler.ServeHTTP(reply, req)
		if reply.Code != want {
			t.Fatalf("heartbeat: status %d, want %d: %s", reply.Code, want, reply.Body)
		}
	}
	at := time.Now().UTC()
	cpu := uint64(0)
	sample := runtimeinfo.Snapshot{Host: &runtimeinfo.Host{SampledAt: at, CPUPercent: &cpu, Workspace: &runtimeinfo.Space{Available: 0, Total: 512}}}
	raw, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	beat(string(raw), http.StatusNoContent)
	reply := read("/workers", "owner", http.StatusOK)
	var nodes []struct {
		Capabilities struct {
			Diagnostics runtimeinfo.Snapshot `json:"diagnostics"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(reply.Body.Bytes(), &nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Capabilities.Diagnostics.Host == nil ||
		nodes[0].Capabilities.Diagnostics.Host.CPUPercent == nil || *nodes[0].Capabilities.Diagnostics.Host.CPUPercent != 0 {
		t.Fatalf("zero-valued sample was lost: %s", reply.Body)
	}
	beat(fmt.Sprintf(`{"host":{"sampled_at":%q,"cpu_percent":101}}`, at.Format(time.RFC3339)), http.StatusNoContent)
	if reply := read("/workers", "owner", http.StatusOK); strings.Contains(reply.Body.String(), `"diagnostics"`) {
		t.Fatalf("invalid diagnostics must not be recorded: %s", reply.Body)
	}
	beat("", http.StatusNoContent)
	if reply := read("/workers", "owner", http.StatusOK); strings.Contains(reply.Body.String(), `"diagnostics"`) {
		t.Fatalf("old heartbeat should be accepted without a sample: %s", reply.Body)
	}
}

func TestProjectOverviewContextScopesActivityAndReadsGit(t *testing.T) {
	ctx := context.Background()
	root, repo, base := issueTestRepo(t)
	db := testdb.Open(t)
	data := store.New(db)
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{project.ID, other.ID} {
		if _, err := db.ExecContext(ctx, `WITH task AS (
			INSERT INTO work_items(project_id,kind,status) VALUES ($1,'git_head','FAILED') RETURNING id
		) INSERT INTO task_events(task_id,type) SELECT id,'execution_failed' FROM task`, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("draft"), 0600); err != nil {
		t.Fatal(err)
	}
	poller := &issuePoller{projectID: project.ID}
	poller.recordStatus(nil)
	poller.recordStatus(errors.New("private diagnostic text"))
	handler := http.NewServeMux()
	registerProjectOverviewContext(handler, data, root, "owner", map[int64]*issuePoller{project.ID: poller})
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/overview-context", project.ID), nil)
	req.Header.Set("Authorization", "Bearer owner")
	reply := httptest.NewRecorder()
	handler.ServeHTTP(reply, req)
	if reply.Code != http.StatusOK {
		t.Fatalf("overview: status %d: %s", reply.Code, reply.Body)
	}
	var page struct {
		Repository     *projectRepositoryStatus `json:"repository"`
		Activity       []store.ProjectActivity  `json:"activity"`
		GitHub         string                   `json:"github"`
		Poll           *githubPollStatus        `json:"poll"`
		ActivityFailed bool                     `json:"activity_failed"`
		CheckFailed    bool                     `json:"check_failed"`
	}
	if err := json.Unmarshal(reply.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.ActivityFailed || page.CheckFailed || len(page.Activity) != 1 || page.Activity[0].Event != "execution_failed" ||
		page.Repository == nil || page.Repository.SHA != base || page.Repository.Dirty == nil || !*page.Repository.Dirty ||
		page.GitHub != "failed" || page.Poll == nil || page.Poll.LastSuccess == nil || strings.Contains(reply.Body.String(), "private diagnostic text") {
		t.Fatalf("unexpected overview response: %s", reply.Body)
	}
}
