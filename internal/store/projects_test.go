package store

import (
	"context"
	"errors"
	"testing"
)

func TestProjectCheckReplaysAcrossSessions(t *testing.T) {
	s := New(isolatedDatabase(t))
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "project")
	if err != nil {
		t.Fatal(err)
	}
	type response struct {
		task Task
		err  error
	}
	results := make(chan response, 2)
	pair := func(retry int64) int64 {
		t.Helper()
		for range 2 {
			go func() { task, err := s.ProjectCheck(ctx, p.ID, retry); results <- response{task, err} }()
		}
		a, b := <-results, <-results
		if a.err != nil || b.err != nil || a.task.ID != b.task.ID || a.task.Status != "READY" {
			t.Fatal(a, b)
		}
		return a.task.ID
	}
	first := pair(0)
	j, err := s.Claim(ctx, "worker", t.TempDir())
	if err != nil || j == nil || j.TaskID != first {
		t.Fatal("missing first check", err)
	}
	if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{ExitCode: 1, Error: "repository unavailable"}); err != nil {
		t.Fatal(err)
	}
	failed, err := s.ProjectCheck(ctx, p.ID, 0)
	if err != nil || failed.ID != first || failed.Status != "FAILED" {
		t.Fatal(failed, err)
	}
	second := pair(first)
	if first == second {
		t.Fatal("failed check not retried")
	}
	if _, err := s.ProjectCheck(ctx, p.ID, second); !errors.Is(err, ErrConflict) {
		t.Fatal("retried an active check", err)
	}
	other, err := s.CreateProject(ctx, "another")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProjectCheck(ctx, other.ID, first); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted foreign retry", err)
	}
	projects, err := s.RegisteredProjects(ctx, other.ID)
	if err != nil || len(projects) != 1 || projects[0].CheckID != second || projects[0].CheckStatus != "READY" {
		t.Fatal(projects, err)
	}
	j, err = s.Claim(ctx, "worker", t.TempDir())
	if err != nil || j == nil || j.TaskID != second {
		t.Fatal("missing second check", err)
	}
	if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{Commit: "1234567890123456789012345678901234567890"}); err != nil {
		t.Fatal(err)
	}
	if third := pair(second); third == second {
		t.Fatal("cannot check the new HEAD")
	}
}
