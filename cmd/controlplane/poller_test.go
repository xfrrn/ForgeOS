package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"forgeos/internal/store"
)

type pollFixture struct {
	importFixture
	received map[int64]int
	observed map[int64]int
	comments []json.RawMessage
	inbox    []store.Discussion
}

func (f *pollFixture) ObserveIssue(_ context.Context, _ int64, issue store.Issue) (int64, error) {
	if f.observed == nil {
		f.observed = map[int64]int{}
	}
	f.observed[issue.Number]++
	return 0, nil
}

func (f *pollFixture) RecordComments(_ context.Context, _ int64, _ store.Issue, page []json.RawMessage) error {
	f.comments = append(f.comments, page...)
	return nil
}

func (f *pollFixture) ReplaceInbox(_ context.Context, _, _ int64, items []store.Discussion) error {
	f.inbox = items
	return nil
}

func (f *pollFixture) ImportIssue(_ context.Context, _ int64, s store.IssueSpec, _ ...store.FeedbackSource) (store.Task, error) {
	f.received[s.Issue.Number]++
	f.spec = &s
	return store.Task{}, nil
}

func TestPollConfig(t *testing.T) {
	if p, err := pollConfig("", ""); err != nil || p != nil {
		t.Fatal("poller must be disabled by default")
	}
	for _, args := range [][2]string{{"0", "docs"}, {"1", ""}, {"", "go-test"}, {"1", "shell"}} {
		if _, err := pollConfig(args[0], args[1]); err == nil {
			t.Errorf("accepted invalid config %v", args)
		}
	}
}

func TestGitHubWebhookRespectsRateLimit(t *testing.T) {
	root, repo, _ := issueTestRepo(t)
	data := &pollFixture{importFixture: importFixture{path: repo}}
	p := &issuePoller{projectID: 3, validation: "docs", wake: make(chan struct{}, 1)}
	calls := make(chan struct{}, 2)
	stop := p.start(context.Background(), data, root, func(context.Context, string, any) error {
		calls <- struct{}{}
		return errGHLimit
	})
	defer stop()
	select {
	case <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("initial scan did not reach GitHub")
	}
	p.wake <- struct{}{}
	select {
	case <-calls:
		t.Fatal("webhook bypassed the rate-limit backoff")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestConfiguredPollers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "poll.json")
	write := func(doc string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`[{"project_id":2,"validation":"auto"},{"project_id":1,"validation":"docs"}]`)
	got, err := configuredPollers("1", "docs", path)
	if err != nil || len(got) != 2 || got[1].validation != "docs" || got[2].validation != "auto" {
		t.Fatal("expected one poller per project", got, err)
	}
	for _, doc := range []string{
		`[{"project_id":1,"validation":"V2"}]`, `[{"project_id":0,"validation":"docs"}]`,
		`[{"project_id":2}]`, `[{"project_id":2,"validation":"off"}]`,
		`[{"project_id":2,"validation":"V2","target_branch":"main"}]`,
	} {
		write(doc)
		if _, err := configuredPollers("1", "docs", path); err == nil {
			t.Errorf("accepted invalid polling configuration %s", doc)
		}
	}
}

func TestPollSelectedIssuesAcrossPages(t *testing.T) {
	root, path, base := issueTestRepo(t)
	data := &pollFixture{importFixture: importFixture{path: path}, received: make(map[int64]int)}
	p, err := pollConfig("3", "docs")
	if err != nil {
		t.Fatal(err)
	}
	var reads int
	endpoint := "repositories/88/issues?state=open&sort=created&direction=asc&per_page=" + strconv.Itoa(pageSize) + "&page="
	get := func(_ context.Context, path string, dst any) error {
		reads++
		if path == "repos/my-org/project" {
			return json.Unmarshal([]byte(`{"id":88,"full_name":"my-org/project"}`), dst)
		}
		start, end := 1, pageSize
		switch path {
		case endpoint + "1", endpoint + "2":
		case endpoint + "3":
			// A changing GitHub list may repeat the last issue on the next page.
			start, end = pageSize, pageSize+1
		default:
			t.Fatalf("unnecessary GitHub request: %s", path)
		}
		page := make([]githubIssue, 0, end-start+1)
		for n := start; n <= end; n++ {
			i := githubIssue{ID: int64(100 + n), Number: int64(n), RepoURL: "https://api.github.com/repos/my-org/project", State: "open",
				HTMLURL: fmt.Sprintf("https://github.com/my-org/project/issues/%d", n), Title: "sample", UpdatedAt: "2026-01-05T16:00:00Z"}
			if n <= 5 || n >= pageSize {
				i.Labels = []struct{ Name string }{{agentLabel}}
			}
			switch n {
			case 1:
				i.PullRequest = json.RawMessage(`{}`)
			case 2:
				i.State = "closed"
			case 3:
				i.RepoURL = "https://api.github.com/repos/another/project"
			case 4:
				i.HTMLURL = "https://github.com/another/project/issues/4"
			}
			page = append(page, i)
		}
		*dst.(*[]githubIssue) = page
		return nil
	}
	for range 2 {
		if err := p.scan(context.Background(), data, root, get); err != nil {
			t.Fatal(err)
		}
	}
	if data.observed[6] != 2 || data.received[6] != 0 {
		t.Fatal("ordinary discussion was dispatched or omitted", data.observed[6], data.received[6])
	}
	if reads != 8 || len(data.inbox) != pageSize-6 || data.inbox[0].Number != 6 || len(data.received) != 3 || data.received[5] != 2 || data.received[int64(pageSize+1)] != 2 ||
		data.spec.BaseCommit != base || data.spec.Validation != "docs" || data.spec.Issue.RepositoryID != 88 {
		t.Fatalf("reads=%d received=%v spec=%+v", reads, data.received, data.spec)
	}
}

func TestPollingStopsDuringRequest(t *testing.T) {
	root, path, _ := issueTestRepo(t)
	p := &issuePoller{projectID: 3, validation: "go-test"}
	data := &pollFixture{importFixture: importFixture{path: path}}
	started := make(chan struct{})
	get := func(ctx context.Context, _ string, _ any) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	stop := p.start(context.Background(), data, root, get)
	select {
	case <-started:
	case <-time.After(4 * time.Second):
		stop()
		t.Fatal("poller did not start")
	}
	stop()
	if err := p.scan(context.Background(), data, root, func(context.Context, string, any) error { return errGHLimit }); !errors.Is(err, errGHLimit) {
		t.Fatalf("expected GitHub rate limit, got %v", err)
	}
}
