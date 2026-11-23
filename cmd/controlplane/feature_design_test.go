package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
	"forgeos/internal/validation"
	"github.com/google/uuid"
)

func TestFeatureDesignDispatchAndReceipt(t *testing.T) {
	for _, scenario := range []string{"new", "existing", "split"} {
		existing := scenario == "existing"
		split := scenario == "split"
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			db := testdb.Open(t)
			s := store.New(db)
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(s.Migrate(ctx))
			root, repo, head := issueTestRepo(t)
			p, err := s.CreateProject(ctx, repo)
			must(err)
			in := store.ProposalRequest{Client: uuid.New(), Lead: uuid.New(), Advisor: uuid.New(), Brief: "test feature"}
			for _, m := range []uuid.UUID{in.Lead, in.Advisor} {
				_, err = db.Exec("INSERT INTO chat_models(id,model,revision) VALUES($1,$2,$3)", m, m.String(), uuid.New())
				must(err)
			}
			f, err := s.CreateFeature(ctx, p.ID, uuid.New(), in.Lead, "筛选")
			must(err)
			round, err := s.StartFeatureProposal(ctx, p.ID, f.ID, in, head)
			must(err)
			complete := func(body string) int64 {
				t.Helper()
				job, err := s.ClaimDiscussionJob(ctx, p.ID, true, "", true)
				must(err)
				if job == nil {
					t.Fatal("missing discussion job")
				}
				must(s.CompleteChat(ctx, p.ID, job.ID, job.Lease, body, "", nil, nil))
				return job.ID
			}
			complete("lead")
			complete("advisor")
			complete("approved summary")
			must(s.ActOnProposal(ctx, p.ID, f.ID, round, store.ProposalAction{Operation: "approve", Revision: 1}))
			complete("draft with interface")
			complete("review with correction")
			const final = "Deliver the tested interface. Acceptance: clearing the filter restores every item."
			answer := final
			if split {
				answer += "\n```forgeos-ui\n" + `{"title":"Filter","regions":[{"label":"Tools","position":"main","elements":[{"id":"query","kind":"input","label":"Query field"}]}]}` + "\n```"
			}
			finalTurn := complete(answer)
			raw := githubIssue{ID: 602, Number: 21, Title: "Filter", Body: "Existing request", State: "open", UpdatedAt: "2026-01-06T10:00:00Z", HTMLURL: "https://github.com/my-org/project/issues/21", RepoURL: "https://api.github.com/repos/my-org/project"}
			get := func(ctx context.Context, path string, out any) error {
				switch path {
				case "repos/my-org/project":
					return json.Unmarshal([]byte(`{"id":88,"full_name":"my-org/project"}`), out)
				case "repositories/88/issues/21":
					*(out.(*githubIssue)) = raw
					return nil
				}
				return fmt.Errorf("unexpected read %s", path)
			}
			posts := 0
			client := func(ctx context.Context, _ store.Project) (deliveryAPI, error) {
				return func(ctx context.Context, method, path string, body, out any) error {
					if method == "GET" && strings.Contains(path, "/issues?") {
						*(out.(*[]githubIssue)) = []githubIssue{raw}
						return nil
					}
					if method == "GET" {
						return get(ctx, path, out)
					}
					if method != "POST" || existing || path != "repos/my-org/project/issues" {
						t.Fatalf("unexpected mutation %s %s", method, path)
					}
					posts++
					draft := body.(map[string]string)
					raw.Title = draft["title"]
					raw.Body = draft["body"]
					if !strings.Contains(raw.Body, final) {
						t.Fatal("published incorrect design")
					}
					return context.DeadlineExceeded
				}, nil
			}
			mux := http.NewServeMux()
			registerProjectChat(mux, s, root, "owner", nil, get, nil, client)
			path := fmt.Sprintf("/projects/%d/features/%d/proposals/%d", p.ID, f.ID, round)
			request := func(method, route string, body any, status int, out any) {
				t.Helper()
				b, _ := json.Marshal(body)
				r := httptest.NewRequest(method, route, bytes.NewReader(b))
				r.Header.Set("Authorization", "Bearer owner")
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if w.Code != status {
					t.Fatalf("%s: HTTP %d %s", route, w.Code, w.Body.String())
				}
				if out != nil {
					must(json.Unmarshal(w.Body.Bytes(), out))
				}
			}
			input := store.DesignDispatch{Turn: finalTurn, Revision: 1}
			if split {
				input.Steps = []store.ImplementationStep{{Name: "UI", Scope: "form", Harness: "codex", Model: "ui-model"}, {Name: "API", Scope: "endpoint", Harness: "claude", Model: "api-model"}}
			}
			if existing {
				input.Issue = 21
			} else {
				request("POST", path+"/dispatch", input, 502, nil)
			}
			var turn store.ChatTurn
			request("POST", path+"/dispatch", input, 200, &turn)
			if turn.Action == nil || turn.Action.TaskID == 0 || turn.Action.Status != "confirmed" {
				t.Fatal("missing task", turn.Action)
			}
			request("POST", path+"/dispatch", input, 200, nil)
			if split {
				altered := input
				altered.Steps = append([]store.ImplementationStep(nil), input.Steps...)
				altered.Steps[0].Model = "different"
				request("POST", path+"/dispatch", altered, 409, nil)
			}
			if (!existing && posts != 1) || (existing && posts != 0) {
				t.Fatal("duplicate creation")
			}
			request("POST", fmt.Sprintf("/projects/%d/chat/actions/%d/confirm", p.ID, turn.Action.ID), chatIssueDraft{Body: "replacement"}, 409, nil)
			job, err := s.Claim(ctx, "design-worker", t.TempDir(), true, true, true, false, true, true, true, true)
			must(err)
			caps := store.WorkerCapabilities{FeatureBuild: true, AgentSettings: true, SupportsCodex: true, SupportsClaude: true, SupportsValidation: true, SupportsDeclaredChecks: true, SupportsRiskPolicy: true, SupportsRework: true, SupportsReworkAnswers: true}
			claim := func(worker string) *store.Job {
				j, e := s.ClaimForProjects(ctx, worker, t.TempDir(), []int64{p.ID}, caps)
				must(e)
				return j
			}
			stage := func(want string) {
				t.Helper()
				list, e := s.Features(ctx, p.ID)
				must(e)
				if len(list) != 1 || list[0].Stage != want {
					t.Fatalf("expected stage %s, got %+v", want, list)
				}
			}
			if split {
				stage("implementation_queued")
			} else {
				stage("implementing")
			}
			var frontend, backend string
			if split {
				if job != nil {
					t.Fatal("old worker claimed a feature build")
				}
				capJSON, _ := json.Marshal(caps)
				_, err = db.Exec("INSERT INTO worker_nodes(worker,os,arch,cpus,slots,capabilities,model_slots) VALUES('design-worker','windows','amd64',2,1,$1,1)", string(capJSON))
				must(err)
				ui := claim("design-worker")
				stage("implementing")
				if ui == nil || ui.Agent.Model != "ui-model" || ui.Spec.FeatureStep.Name != "UI" || !strings.Contains(ui.Spec.Issue.Body, "Query field") {
					t.Fatal("missing frontend assignment", ui)
				}
				if claim("other-node") != nil || claim("design-worker") != nil {
					t.Fatal("node or single-slot isolation failed")
				}
				_, err = db.Exec("UPDATE worker_nodes SET slots=2 WHERE worker='design-worker'")
				must(err)
				api := claim("design-worker")
				if api == nil || api.Spec.Harness != "claude" || api.Agent.Model != "api-model" || api.Workspace == ui.Workspace || api.Spec.BaseCommit != head {
					t.Fatal("missing backend assignment", api)
				}
				done := func(j *store.Job, sha string) {
					r := store.Result{Commit: sha, BaseCommit: head, SessionID: uuid.NewString()}
					if j.Spec.Harness == "claude" {
						r.Harness = "claude"
						r.HarnessVersion = "2.1.0"
					}
					must(s.Complete(ctx, j.ExecutionID, j.LeaseToken, r))
				}
				writeStep := func(name, body string) string {
					t.Helper()
					dir := filepath.Join(t.TempDir(), "step")
					deliveryGit(t, repo, "worktree", "add", "--detach", dir, head)
					must(os.Mkdir(filepath.Join(dir, "dist"), 0700))
					must(os.WriteFile(filepath.Join(dir, "dist", name), []byte(body), 0600))
					deliveryGit(t, dir, "add", "dist")
					deliveryGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", name)
					return deliveryGit(t, dir, "rev-parse", "HEAD")
				}
				frontend = writeStep("index.html", "<h1>Feature preview</h1>")
				done(ui, frontend)
				must(s.Complete(ctx, api.ExecutionID, api.LeaseToken, store.Result{ExitCode: 1, Error: "backend interrupted"}))
				if claim("design-worker") != nil {
					t.Fatal("released integration on failure")
				}
				stage("implementation_failed")
				_, err = s.RetryIssueExecution(ctx, api.ExecutionID)
				must(err)
				retry := claim("design-worker")
				if retry == nil || retry.TaskID != api.TaskID {
					t.Fatal("wrong retry")
				}
				done(retry, writeStep("index.html", "conflicting API fixture"))
				stage("integration_queued")
				job = claim("design-worker")
				if job == nil || len(job.FeatureInputs) != 2 || job.FeatureInputs[0].Execution != ui.ExecutionID || job.FeatureInputs[1].Execution != retry.ExecutionID {
					t.Fatal("incorrect integration manifest", job)
				}
				stage("integrating")
				must(s.Complete(ctx, job.ExecutionID, job.LeaseToken, store.Result{ExitCode: 1, Error: "merge conflict"}))
				stage("integration_failed")
				request("POST", path+"/revise-step", map[string]any{"task": api.TaskID, "execution": job.ExecutionID, "note": "Resolve shared contract"}, 200, nil)
				revised := claim("design-worker")
				stage("reworking")
				if revised == nil || revised.TaskID != api.TaskID || !strings.Contains(revised.Spec.Issue.Body, "Resolve shared contract") {
					t.Fatal("wrong rework")
				}
				backend = writeStep("contract.json", "{\"filter\":true}")
				done(revised, backend)
				job = claim("design-worker")
				if job == nil || job.FeatureInputs[0].Execution != ui.ExecutionID || job.FeatureInputs[1].Execution != revised.ExecutionID {
					t.Fatal("lost sibling after revision")
				}
			}
			if job == nil || job.TaskID != turn.Action.TaskID || !strings.Contains(job.Spec.Issue.Body, final) {
				t.Fatal("worker did not receive final design")
			}
			if !existing {
				verify := func(context.Context, string, store.IssueSpec, store.Result) (bool, error) { return false, nil }
				finish := func(candidate string) {
					t.Helper()
					now := time.Now().UTC()
					run := validation.Run{ID: "ci-run-1", Runner: "design-worker", ExecutionID: job.ExecutionID, Base: head, Commit: candidate, Level: "V1", Decision: &validation.Decision{Policy: *job.Spec.Policy, Level: "V1", Base: head, Commit: candidate}, Started: now, Finished: now, Checks: []validation.CheckResult{}}
					result := store.Result{Commit: candidate, BaseCommit: head, SessionID: uuid.NewString(), Validation: &store.ValidationResult{Commit: candidate, Profile: "auto", Run: &run}}
					if split && job.Rework == nil {
						result.SessionID = ""
						must(s.RecordCICandidate(ctx, job.ExecutionID, job.LeaseToken, candidate, ""))
					}
					must(s.CompleteWithChecks(ctx, job.ExecutionID, job.LeaseToken, result, verify))
				}
				receiptContains := func(parts ...string) {
					t.Helper()
					var r store.DesignReceipt
					request("GET", path+"/delivery", nil, 200, &r)
					for _, part := range parts {
						if !strings.Contains(string(r.Result), part) {
							t.Fatalf("missing %q in %s", part, r.Result)
						}
					}
				}
				first := strings.Repeat("a", 40)
				if split {
					tree := deliveryGit(t, repo, "merge-tree", "--write-tree", frontend, backend)
					first = deliveryGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit-tree", tree, "-p", head, "-m", "preview fixture")
				}
				finish(first)
				stage("acceptance")
				preview, err := s.OpenPreview(ctx, job.TaskID, job.ExecutionID, first, "dist", 18001, 120)
				must(err)
				if split {
					var info struct{ ID string }
					must(json.Unmarshal(preview, &info))
					serve := previewAssets(s, root, 18001, nil, "")
					for route, want := range map[string]string{"/": "<h1>Feature preview</h1>", "/contract.json": "{\"filter\":true}"} {
						req := httptest.NewRequest("GET", "http://p-"+info.ID+".localhost:18001"+route, nil)
						res := httptest.NewRecorder()
						serve(res, req)
						if res.Code != 200 || res.Body.String() != want || res.Header().Get("X-ForgeOS-Commit") != first {
							t.Fatalf("preview %s did not serve combined commit: %d %s", route, res.Code, res.Body.String())
						}
					}
				}
				_, err = s.Review(ctx, job.TaskID, job.ExecutionID, first, "reject", "Revise the empty state", false, "")
				must(err)
				receiptContains(first, "Revise the empty state", "active")
				stage("rework_queued")
				if split {
					receiptContains("ui-model", "api-model", frontend, backend, "inputs")
				}
				job, err = s.Claim(ctx, "design-worker", t.TempDir(), true, true, true, false, true, true, true, true)
				must(err)
				if split {
					job = claim("design-worker")
				}
				if job == nil || job.TaskID != turn.Action.TaskID {
					t.Fatal("missing rework")
				}
				second := strings.Repeat("b", 40)
				finish(second)
				_, err = s.Review(ctx, job.TaskID, job.ExecutionID, second, "approve", "Verified", true, "main")
				must(err)
				delivery, err := s.ClaimDelivery(ctx, p.ID)
				must(err)
				if delivery == nil {
					t.Fatal("missing delivery")
				}
				delivery.PRNumber = 12
				delivery.PRURL = "https://github.com/my-org/project/pull/12"
				delivery.MergeSHA = strings.Repeat("c", 40)
				delivery.State = "DONE"
				must(s.SaveDelivery(ctx, *delivery))
				stage("merged")
				receiptContains(second, "Verified", "Revise the empty state", delivery.MergeSHA, "pr_number")
				return
			}
			must(s.Complete(ctx, job.ExecutionID, job.LeaseToken, store.Result{ExitCode: 1, Error: "isolated test result", BaseCommit: job.Spec.BaseCommit}))
			var receipt store.DesignReceipt
			request("GET", path+"/delivery", nil, 200, &receipt)
			if receipt.FinalTurn != finalTurn || receipt.Action.TaskID != job.TaskID || !strings.Contains(string(receipt.Result), "isolated test result") || !strings.Contains(string(receipt.Progress), "FAILED") || len(receipt.Activities) == 0 {
				t.Fatalf("incomplete receipt: %+v", receipt)
			}
		})
	}
}
