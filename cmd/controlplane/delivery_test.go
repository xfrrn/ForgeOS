package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestDeliveryConfig(t *testing.T) {
	for _, tc := range []struct {
		id, target string
		valid      bool
	}{
		{"", "", true}, {"", "main", false}, {"0", "", false}, {"1", "", true},
		{"2", "release/v1", true}, {"2", "HEAD", false}, {"2", "bad..branch", false},
	} {
		_, err := deliveryConfig(tc.id, tc.target)
		if (err == nil) != tc.valid {
			t.Errorf("deliveryConfig(%q, %q): %v", tc.id, tc.target, err)
		}
	}
}

func TestConfiguredDeliveryProjects(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`[{"project_id":2,"target_branch":"release/v1"},{"project_id":3}]`)
	targets, err := configuredDeliveryTargets("1", "main", file)
	if err != nil || len(targets) != 3 || targets[1] != "main" || targets[2] != "release/v1" {
		t.Fatal("expected separate delivery policies", targets, err)
	}
	if target, ok := targets[3]; !ok || target != "" {
		t.Fatal("missing repository-default target")
	}
	for _, body := range []string{
		`[{"project_id":1,"target_branch":"other"}]`,
		`[{"project_id":2,"target_branch":"bad..branch"}]`,
		`[{"project_id":0}]`, `[{"project_id":2,"branch":"main"}]`, `null`, `[] []`,
	} {
		write(body)
		if _, err := configuredDeliveryTargets("1", "main", file); err == nil {
			t.Errorf("accepted invalid policies: %s", body)
		}
	}
}

func TestAcceptanceToDelivery(t *testing.T) {
	db := testdb.Open(t)
	data := store.New(db)
	ctx := context.Background()
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root, repo, base := issueTestRepo(t)
	project, err := data.CreateProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	policy := deliveryPolicy{projectID: project.ID, target: "main"}
	mux := routes(data, root, "owner", legacyWorker("worker"), deliveryTargets{policy.projectID: policy.target})
	post := func(path, token string, body any, status int) {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s: HTTP %d, expected %d: %s", path, w.Code, status, w.Body.String())
		}
	}
	spec := store.IssueSpec{Issue: store.Issue{ID: 15, RepositoryID: 10, Repository: "my-org/project", Number: 2, Title: "Fix README"}, BaseCommit: base, Validation: "docs"}
	task, err := data.ImportIssue(ctx, project.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	job, err := data.Claim(ctx, "worker", t.TempDir(), true)
	if err != nil || job == nil {
		t.Fatalf("claim: %v", err)
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := localrepo.Git(ctx, dir, args...)
		if err != nil {
			t.Fatalf("git %s: %v", args[0], err)
		}
		return strings.TrimSpace(string(out))
	}
	git(repo, "worktree", "add", "--detach", job.Workspace, base)
	if err := os.WriteFile(filepath.Join(job.Workspace, "README.md"), []byte("verified\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(job.Workspace, "add", "README.md")
	git(job.Workspace, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", "Fix README")
	candidate := git(job.Workspace, "rev-parse", "HEAD")
	post("/internal/executions/"+strconv.FormatInt(job.ExecutionID, 10)+"/result", "worker", map[string]any{
		"lease_token": job.LeaseToken, "exit_code": 0, "session_id": "test-session", "base_commit": base, "commit": candidate,
		"validation": map[string]any{"profile": "docs", "commit": candidate, "exit_code": 0, "output": "pass"},
	}, 204)
	path := "/tasks/" + strconv.FormatInt(task.ID, 10) + "/acceptance"
	body := map[string]any{"execution_id": job.ExecutionID, "commit": base, "decision": "approve"}
	post(path, "owner", body, 409)
	body["commit"] = candidate
	questions := "/tasks/" + strconv.FormatInt(task.ID, 10) + "/questions"
	raw, err := data.OpenPreview(ctx, task.ID, job.ExecutionID, candidate, "public", 8091, 60)
	if err != nil {
		t.Fatal(err)
	}
	var preview struct{ ID string }
	if err = json.Unmarshal(raw, &preview); err != nil {
		t.Fatal(err)
	}
	fid, err := data.SubmitPreviewFeedback(ctx, task.ID, preview.ID, store.PreviewFeedback{Request: "73424164-fd76-4696-afdd-8915a903520f", Execution: job.ExecutionID, Commit: candidate, Path: "/", Note: "Which example is required?"})
	if err != nil {
		t.Fatal(err)
	}
	request := store.QuestionRequest{ExecutionID: job.ExecutionID, Commit: candidate, Key: "feedback-" + strconv.FormatInt(fid, 10), Question: "Which wording?", Evidence: "README diff"}
	post(questions, "owner", request, 200)
	post(questions, "owner", request, 200)
	held, err := data.Task(ctx, task.ID)
	if err != nil || held.Status != "WAITING_INPUT" || len(held.Questions) != 1 {
		t.Fatalf("checkpoint: %+v %v", held, err)
	}
	post(path, "owner", body, 409)
	answerPath := questions + "/" + strconv.FormatInt(held.Questions[0].ID, 10) + "/answer"
	answer := map[string]string{"answer": "Keep the tested example"}
	post(answerPath, "worker", answer, 401)
	post(answerPath, "owner", answer, 200)
	post(answerPath, "owner", answer, 200)
	post(answerPath, "owner", map[string]string{"answer": "different"}, 409)
	resumed, err := data.Task(ctx, task.ID)
	if err != nil || resumed.Status != "WAITING_ACCEPTANCE" || resumed.Execution.ID != job.ExecutionID ||
		resumed.Execution.Result.Commit != candidate || resumed.Questions[0].AnsweredBy != "owner" || len(resumed.Events) != len(held.Events)+1 {
		t.Fatalf("resumed checkpoint: %+v %v", resumed, err)
	}
	t.Run("classified feedback preserves independent questions and evidence", func(t *testing.T) {
		request.FeedbackID = fid
		request.Evidence = "Which example is required?"
		post(questions, "owner", request, 409)
		j, err := data.ClaimFeedback(ctx, project.ID)
		if err != nil || j == nil {
			t.Fatalf("claim: %v", err)
		}
		err = data.ClassifyFeedback(ctx, project.ID, fid, j.Lease, store.FeedbackClassification{Category: "blocked", Rationale: "Scope is unclear.", Evidence: request.Evidence, Session: "management-test"})
		if err != nil {
			t.Fatal(err)
		}
		bad := request
		bad.Evidence = "different quote"
		post(questions, "owner", bad, 409)
		post(questions, "owner", request, 200)
		post(questions, "owner", request, 200)
		held, err := data.Task(ctx, task.ID)
		if err != nil || len(held.Questions) != 2 || held.Questions[1].FeedbackID != fid || held.Status != "WAITING_INPUT" {
			t.Fatalf("feedback checkpoint: %+v %v", held, err)
		}
		post(questions+"/"+strconv.FormatInt(held.Questions[1].ID, 10)+"/answer", "owner", answer, 200)
		post(questions, "owner", request, 200)
	})
	post(path, "worker", body, 401)
	post(path, "owner", body, 200)
	post(path, "owner", body, 200)

	// Only the GitHub boundary is simulated here. The acceptance, queue and audit use real PostgreSQL.
	states := []string{"WAITING", "MERGED", "DONE"}
	mergeSHA := strings.Repeat("b", 40)
	for _, state := range states {
		claimed, err := policy.step(ctx, store.New(db), root, func(_ context.Context, dir string, frozen store.Task, d store.Delivery) (store.Delivery, error) {
			if dir != repo || frozen.Execution.Result.Commit != candidate || d.Commit != candidate || d.ExecutionID != job.ExecutionID {
				t.Fatal("lost frozen delivery identity")
			}
			d.State = state
			switch state {
			case "WAITING":
				d.PRNumber, d.PRURL = 42, "https://github.com/my-org/project/pull/42"
			case "MERGED":
				d.MergeSHA = mergeSHA
			}
			return d, nil
		})
		if err != nil || !claimed {
			t.Fatalf("deliver %s: claimed=%t err=%v", state, claimed, err)
		}
	}
	final, err := store.New(db).Task(ctx, task.ID)
	if err != nil || final.Status != "DONE" || final.Delivery.MergeSHA != mergeSHA || len(final.Reviews) != 1 {
		t.Fatalf("final: %+v, %v", final, err)
	}
	post(path, "owner", body, 200)
	extra, err := data.ClaimDelivery(ctx, project.ID)
	if err != nil || extra != nil {
		t.Fatalf("completed delivery claimed again: %v", err)
	}
}
