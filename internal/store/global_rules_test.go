package store

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGlobalRulesPublishOneImmutableRevision(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if doc, err := s.GlobalRules(ctx, 0); err != nil || string(doc) != "null" {
		t.Fatal(string(doc), err)
	}
	type result struct {
		content string
		receipt string
		err     error
	}
	results := make(chan result, 2)
	for _, content := range []string{"Default A", "Default B"} {
		go func() {
			doc, err := s.PublishGlobalRules(ctx, 0, content)
			results <- result{content, string(doc), err}
		}()
	}
	first, other := <-results, <-results
	if first.err != nil {
		first, other = other, first
	}
	if first.err != nil || !errors.Is(other.err, ErrConflict) {
		t.Fatal(first.err, other.err)
	}
	if doc, err := s.PublishGlobalRules(ctx, 0, first.content); err != nil || string(doc) != first.receipt {
		t.Fatal(string(doc), err)
	}
	if _, err := s.PublishGlobalRules(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}
	if doc, err := s.GlobalRules(ctx, 1); err != nil || string(doc) != first.receipt {
		t.Fatal(string(doc), err)
	}
	if doc, err := s.GlobalRules(ctx, 0); err != nil || strings.Contains(string(doc), first.content) {
		t.Fatal("withdrawn default still current", err)
	}
	if _, err := s.PublishGlobalRules(ctx, 4, "future"); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted missing previous revision", err)
	}
}

func TestGlobalRulesFrozenAtModelClaim(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	importIssue := func(id int64) Task {
		t.Helper()
		task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 1, ID: id, Number: id}, BaseCommit: strings.Repeat("a", 40), Validation: "docs", Harness: "codex"})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	first := importIssue(1)
	revision, err := s.PublishGlobalRules(ctx, 0, "Default A")
	if err != nil {
		t.Fatal(err)
	}
	h := WorkerHeartbeat{Worker: "rules", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, Capabilities: WorkerCapabilities{SupportsCodex: true}}
	root := t.TempDir()
	claim := func(advertised, requested bool) *Job {
		t.Helper()
		h.Capabilities.SupportsGlobalRules = advertised
		if err := s.RecordHeartbeat(ctx, h); err != nil {
			t.Fatal(err)
		}
		caps := h.Capabilities
		caps.SupportsGlobalRules = requested
		j, err := s.ClaimForProjects(ctx, h.Worker, root, []int64{p.ID}, caps)
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	if claim(false, true) != nil || claim(true, false) != nil {
		t.Fatal("incompatible worker claimed active defaults")
	}
	j := claim(true, true)
	if j == nil || j.TaskID != first.ID || !bytes.Equal(j.GlobalRules, revision) {
		t.Fatal("missing published defaults")
	}
	if _, err = s.PublishGlobalRules(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{ExitCode: 1, Error: "fixture completed"}); err != nil {
		t.Fatal(err)
	}
	task, err := s.Task(ctx, first.ID)
	if err != nil || task.Execution.GlobalRules == nil || !strings.Contains(string(*task.Execution.GlobalRules), "Default A") {
		t.Fatal("lost claim receipt", err)
	}
	second := importIssue(2)
	j = claim(false, false)
	if j == nil || j.TaskID != second.ID || j.GlobalRules != nil {
		t.Fatal("legacy claim after withdrawal")
	}
	task, err = s.Task(ctx, second.ID)
	if err != nil || task.Execution.GlobalRules != nil {
		t.Fatal("withdrawn defaults in execution history", err)
	}
}
