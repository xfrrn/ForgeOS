package main

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
	"forgeos/internal/validation"
)

func TestDeclaredChecksRequiredForIssue(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(data.Migrate(ctx))
	root, repo, _ := issueTestRepo(t)
	update := func(body string) string {
		t.Helper()
		must(os.WriteFile(filepath.Join(repo, "validation.yaml"), []byte(body), 0600))
		_, err := localrepo.Git(ctx, repo, "add", "validation.yaml")
		must(err)
		_, err = localrepo.Git(ctx, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "validation contract")
		must(err)
		head, err := localrepo.Git(ctx, repo, "rev-parse", "HEAD")
		must(err)
		return strings.TrimSpace(string(head))
	}
	cfg := "version: 1\nchecks:\n  format: {type: static, command: [gofmt, -l, .], timeout_seconds: 30}\nprofiles:\n  fast: {checks: [format]}\n"
	base := update(cfg)
	candidate := update(strings.Replace(cfg, "gofmt, -l, .", "false", 1))
	project, err := data.CreateProject(ctx, repo)
	must(err)
	task, err := data.ImportIssue(ctx, project.ID, store.IssueSpec{Issue: store.Issue{ID: 1, Number: 1, RepositoryID: 2}, BaseCommit: base, Validation: "V1"})
	must(err)
	old, err := data.Claim(ctx, "old", t.TempDir(), true, true, true, true, true)
	must(err)
	if old != nil {
		t.Fatal("legacy node claimed declared checks")
	}
	job, err := data.Claim(ctx, "runner", t.TempDir(), true, true, true, false, true, true)
	must(err)
	if job == nil {
		t.Fatal("missing issue job")
	}
	plan, err := validation.ReadPlan(ctx, repo, base, "V1")
	must(err)
	now := time.Now().UTC()
	code := 0
	run := validation.Run{ID: "ci-run-42", Runner: "runner", ExecutionID: job.ExecutionID, Base: base, Commit: candidate, ConfigBlob: plan.ConfigBlob, ConfigVersion: 1, Level: "V1", Profile: "fast", Image: "sha256:" + strings.Repeat("a", 64), Started: now, Finished: now}
	result := store.Result{Commit: candidate, BaseCommit: base, SessionID: "dev", Validation: &store.ValidationResult{Profile: "V1", Commit: candidate, Run: &run}}
	verify := verifyDeclaredChecks(root)
	complete := func() error { return data.CompleteWithChecks(ctx, job.ExecutionID, job.LeaseToken, result, verify) }
	conflict := func(err error) {
		t.Helper()
		if !errors.Is(err, store.ErrConflict) {
			t.Fatalf("expected conflict; got %v", err)
		}
	}
	conflict(complete())
	run.Checks = []validation.CheckResult{{Name: "format", Log: "format.log", Started: now, Finished: now, Status: "PASSED", ExitCode: &code}}
	conflict(complete())
	run.Checks[0].Required = true
	run.Runner = "other"
	conflict(complete())
	run.Runner = "runner"
	run.ExecutionID++
	conflict(complete())
	run.ExecutionID--
	run.ConfigBlob = candidate
	conflict(complete())
	run.ConfigBlob = plan.ConfigBlob
	policy := validation.Policy{Minimum: "V1", Target: "pull_request"}
	forged, err := validation.Assess(ctx, repo, base, candidate, policy)
	must(err)
	if forged.Level != "V4" {
		t.Fatal("policy file did not trigger critical checks")
	}
	forged.Level = "V1"
	run.Decision = &forged
	_, err = verify(ctx, repo, store.IssueSpec{Validation: "auto", BaseCommit: base, Policy: &policy}, result)
	conflict(err)
	run.Decision = nil
	content := "format passed\n"
	digest := validation.DigestBytes([]byte(content))
	run.Checks[0].LogDigest = &digest
	must(complete())
	must(complete())
	t.Run("read the accepted file by task and execution", func(t *testing.T) {
		t.Setenv("FORGEOS_ARTIFACT_ROOT", t.TempDir())
		api := routes(data, root, "owner", legacyWorker("runner"), nil)
		file := fmt.Sprintf("/tasks/%d/executions/%d/ci/%s/format.log", task.ID, job.ExecutionID, run.ID)
		do := func(method, path, token, body string, status int) *httptest.ResponseRecorder {
			t.Helper()
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			got := httptest.NewRecorder()
			api.ServeHTTP(got, req)
			if got.Code != status {
				t.Fatalf("%s %s: %d (expected %d)", method, path, got.Code, status)
			}
			return got
		}
		do("GET", file, "owner", "", 503)
		do("PUT", "/internal"+file, "runner", strings.Repeat("x", len(content)), 422)
		do("PUT", "/internal"+file, "runner", content, 204)
		do("PUT", "/internal"+file, "runner", content, 204)
		got := do("GET", file, "owner", "", 200)
		if got.Body.String() != content || got.Header().Get("Content-Disposition") != `attachment; filename="format.log"` {
			t.Fatal("invalid archive download")
		}
		do("GET", file, "runner", "", 401)
		do("GET", strings.Replace(file, run.ID, "ci-run-999", 1), "owner", "", 404)
		do("PUT", "/internal"+file, "runner", content+"x", 422)
		do("PUT", "/internal"+strings.Replace(file, "format.log", "extra.bin", 1), "runner", content, 404)
		retention := fmt.Sprintf("/projects/%d/ci-retention", project.ID)
		do("PUT", retention, "runner", `{"log_seconds":1}`, 401)
		do("PUT", retention, "owner", `{"log_seconds":-1}`, 400)
		do("PUT", retention, "owner", "null", 400)
		do("HEAD", "/internal"+file, "runner", "", 204)
		_, err = db.Exec("UPDATE ci_archive_files SET confirmed_at=now()-interval '2 days' WHERE execution_id=$1", job.ExecutionID)
		must(err)
		do("PUT", retention, "owner", `{"artifact_seconds":10}`, 200)
		do("GET", file, "owner", "", 200)
		locked, release := make(chan struct{}), make(chan struct{})
		upload, policy := make(chan error, 1), make(chan error, 1)
		go func() {
			upload <- data.AccessCIFile(ctx, task.ID, job.ExecutionID, run.ID, "format.log", func(validation.FileDigest) error {
				close(locked)
				<-release
				return saveArtifact(fmt.Sprintf("ci/%d/%d/%s/format.log", task.ID, job.ExecutionID, run.ID), []byte(content))
			})
		}()
		select {
		case <-locked:
		case err := <-upload:
			t.Fatalf("could not open upload: %v", err)
		}
		go func() {
			_, err := data.ProjectCIRetention(ctx, project.ID, &store.CIRetention{LogSeconds: 86400})
			policy <- err
		}()
		select {
		case err := <-policy:
			close(release)
			t.Fatalf("policy overtook an in-flight upload: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		must(<-upload)
		must(<-policy)
		do("GET", file, "owner", "", 410)
		do("HEAD", "/internal"+file, "runner", "", 410)
		do("PUT", retention, "owner", `{}`, 200)
		do("PUT", "/internal"+file, "runner", content, 410)
		busy := errors.New("storage offline")
		if err := data.PurgeCIFiles(ctx, func(store.CIArchiveFile) error { return busy }); !errors.Is(err, busy) {
			t.Fatalf("cleanup lost storage failure: %v", err)
		}
		must(data.PurgeCIFiles(ctx, removeCIFile))
		name, err := artifactPath(fmt.Sprintf("ci/%d/%d/%s/format.log", task.ID, job.ExecutionID, run.ID))
		must(err)
		if _, err = os.Stat(name); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expired bytes remain: %v", err)
		}
		must(data.PurgeCIFiles(ctx, func(store.CIArchiveFile) error { return errors.New("removed twice") }))
	})
	tk, err := data.Task(ctx, task.ID)
	must(err)
	if tk.Status != "WAITING_ACCEPTANCE" || tk.Execution.CodeReviewState != "pending" {
		t.Fatalf("edited validation configuration accepted without review: %+v", tk)
	}
	_, err = data.Review(ctx, tk.ID, job.ExecutionID, candidate, "approve", "", false, "")
	conflict(err)
}

func TestIssueRiskCannotBeLoweredByReceipt(t *testing.T) {
	ctx := context.Background()
	root, repo, base := issueTestRepo(t)
	// Introducing the project policy changes a protected file, even for a docs request.
	if err := os.WriteFile(filepath.Join(repo, "acceptance.yaml"), []byte("version: 1\ncomponents: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "acceptance.yaml"}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "declare acceptance policy"}} {
		if _, err := localrepo.Git(ctx, repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	out, err := localrepo.Git(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	candidate := strings.TrimSpace(string(out))
	spec := (githubProject{Base: base, Declared: true}).spec(store.Issue{}, "docs", "", validation.Policy{Target: "local"})
	if spec.Validation != "auto" || spec.Policy.Minimum != "V1" || spec.Policy.Target != "pull_request" {
		t.Fatal(spec)
	}
	actual, err := validation.Assess(ctx, repo, base, candidate, *spec.Policy)
	if err != nil || actual.Level != "V4" {
		t.Fatal(actual, err)
	}
	result := store.Result{Commit: candidate, Validation: &store.ValidationResult{Profile: "auto", Run: &validation.Run{Decision: &actual, Level: "V1"}}}
	verify := verifyDeclaredChecks(root)
	for _, level := range []string{"V1", "V4"} {
		actual.Level, result.Validation.Run.Level = level, level
		if _, err := verify(ctx, repo, spec, result); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("accepted %s without its complete fixed plan: %v", level, err)
		}
	}
}
