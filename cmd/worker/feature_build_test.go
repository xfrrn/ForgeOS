package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/store"
)

func TestFeatureBuildAssemblesAndValidatesFrozenSiblings(t *testing.T) {
	w, job, base := fixture(t)
	ctx := context.Background()
	job.Kind = "codex_issue"
	job.Spec = &store.IssueSpec{FeatureIntegration: true, BaseCommit: base, Validation: "docs"}
	makeStep := func(id int64, file, body string) store.FeatureInput {
		t.Helper()
		dir := filepath.Join(w.workspaceRoot, file+body)
		if _, err := checkedWorktree(ctx, job.RepoPath, dir, base); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "docs"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docs", file), []byte(body+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "add", "docs")
		tree := git(t, dir, "write-tree")
		sha, err := createCandidateCommit(ctx, dir, tree, base, 1)
		if err != nil {
			t.Fatal(err)
		}
		child := store.Job{TaskID: id, ExecutionID: id + 100, Spec: &store.IssueSpec{FeatureStep: &store.FeatureStep{Parent: job.TaskID}}}
		if err = pinFeatureStep(ctx, dir, child, sha); err != nil {
			t.Fatal(err)
		}
		if err = pinFeatureStep(ctx, dir, child, sha); err != nil {
			t.Fatal(err)
		}
		return store.FeatureInput{Task: id, Execution: child.ExecutionID, Name: body, Commit: sha}
	}
	a := makeStep(1, "front.md", "frontend")
	b := makeStep(2, "back.md", "backend")
	job.FeatureInputs = []store.FeatureInput{a, b}
	r := w.execute(ctx, job)
	if r.Error != "" || r.Validation == nil || r.Validation.ExitCode != 0 || r.Commit != r.Validation.Commit || r.SessionID != "" {
		t.Fatalf("integration result %+v", r)
	}
	if git(t, job.RepoPath, "rev-parse", "HEAD") != base || git(t, job.RepoPath, "rev-list", "-1", "--parents", r.Commit) != r.Commit+" "+base {
		t.Fatal("wrong commit ancestry")
	}
	for _, p := range []string{"front.md", "back.md"} {
		if _, err := os.Stat(filepath.Join(job.Workspace, "docs", p)); err != nil {
			t.Fatal(err)
		}
	}
	c := makeStep(3, "front.md", "incompatible")
	job.FeatureInputs = []store.FeatureInput{a, c}
	if _, err := assembleFeature(ctx, job.RepoPath, job); err == nil || !strings.Contains(err.Error(), "front.md") {
		t.Fatalf("missing conflict evidence: %v", err)
	}
	job.FeatureInputs[1].Commit = b.Commit
	if _, err := assembleFeature(ctx, job.RepoPath, job); err == nil {
		t.Fatal("accepted mismatched receipt")
	}
}
