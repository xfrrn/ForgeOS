package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

func TestProbeCLIOptionDeclarations(t *testing.T) {
	help := []byte("  -p, --print   Start a turn\n      --resume-file <path>   Used in place of --resume\n      --json\n")
	if !hasCLIOptions(help, "-p", "--print", "--json") || hasCLIOptions(help, "--resume") {
		t.Fatal("probe mixed an option prefix or description with a supported option")
	}
}

func TestImplementationPromptUsesOnlyAssignedIssueAndGoalEvidence(t *testing.T) {
	job := store.Job{Spec: &store.IssueSpec{Issue: store.Issue{Title: "Assigned file"}}, LeaseToken: "private-lease-token"}
	for _, evidence := range []json.RawMessage{nil, json.RawMessage(`{"plan_revision":2,"dependencies":[{"key":"guide"}]}`)} {
		job.GoalContext = evidence
		if evidence != nil {
			job.GlobalRules = json.RawMessage(`{"revision":1,"content":"Default heading"}`)
			job.OwnerClarifications = json.RawMessage(`[{"id":7,"execution_id":2,"question":"Which title?","answer":"Harbor"}]`)
		}
		prompt, err := issuePrompt(job, "Keep the heading", store.CheckpointPage{}, nil)
		if err != nil || strings.Contains(prompt, job.LeaseToken) {
			t.Fatal("invalid implementation prompt", err)
		}
		_, payload, found := strings.Cut(prompt, "Issue JSON:\n")
		var source struct {
			Issue    store.Issue     `json:"issue"`
			Rules    json.RawMessage `json:"global_rules"`
			Goal     json.RawMessage `json:"goal_context"`
			Feedback string          `json:"acceptance_feedback"`
			Answers  json.RawMessage `json:"prior_owner_clarifications"`
		}
		if !found || json.Unmarshal([]byte(payload), &source) != nil || source.Issue != job.Spec.Issue ||
			string(source.Rules) != string(job.GlobalRules) || string(source.Goal) != string(evidence) || source.Feedback != "Keep the heading" ||
			string(source.Answers) != string(job.OwnerClarifications) {
			t.Fatalf("incorrect sources: %s", payload)
		}
	}
}

func TestOnlyFinalMessageCanRequestOwnerDecision(t *testing.T) {
	var q nativeQuestion
	q.read(`{"forgeos_question":"Which title?","evidence":"No title in the issue."}`)
	if !q.requested || q.Question != "Which title?" || q.Evidence == "" {
		t.Fatal(q)
	}
	q.read("Implemented the heading.")
	if q.requested || q.Question != "" {
		t.Fatal("reused earlier assistant text", q)
	}
	q.read(`{"forgeos_question":"Which title?","evidence":24}`)
	if _, err := (&worker{}).waitForOwner(context.Background(), store.Job{}, 0, "native", q, nil); !q.requested || err == nil {
		t.Fatal("accepted malformed question", q)
	}
}

func TestCuratorRetriesNoncontiguousSourceQuote(t *testing.T) {
	w := worker{codex: os.Args[0]}
	out, err := w.extractFacts(context.Background(), store.CurationJob{Material: json.RawMessage(`{"content":"SIMULATED_QUOTE_REPAIR: saved 918 then resumed at 918"}`)})
	if err != nil || len(out.Facts) != 1 || out.Facts[0].Quote != "resumed at 918" || out.Session == "" {
		t.Fatal(out, err)
	}
	if _, err := w.extractFacts(context.Background(), store.CurationJob{Material: json.RawMessage(`{"content":"SIMULATED_QUOTE_REPAIR: no quoted offset"}`)}); err == nil {
		t.Fatal("accepted a quote missing from the source after the retry")
	}
}

func TestManagementReviewSendsOnlyFrozenRuleContext(t *testing.T) {
	w := worker{codex: os.Args[0]}
	for _, evidence := range []json.RawMessage{nil, json.RawMessage(`{"global_rules":{"revision":2,"content":"Default heading"},"decisions":[{"id":3,"plan_keys":["guide"],"statement":"Project heading"}]}`)} {
		job := store.ManagementJob{Lease: "private-review-lease", MinimumLevel: "D2", ReviewContext: evidence, Spec: store.IssueSpec{Issue: store.Issue{Title: "SIMULATED_PLAN_REVIEW_CONTEXT"}}}
		r, err := w.reviewManagement(context.Background(), job)
		want := string(evidence)
		if evidence == nil {
			want = "no-context"
		}
		if err != nil || r.Evidence != want || r.SessionID == "" {
			t.Fatal("lost frozen review sources", r, err)
		}
	}
}

func TestManagementAdviceSendsFrozenQuestionEvidence(t *testing.T) {
	w := worker{codex: os.Args[0]}
	for _, evidence := range []json.RawMessage{nil, json.RawMessage(`{"global_rules":{"revision":1,"content":"Fallback"},"owner_clarifications":[{"answer":"Owner heading"}]}`)} {
		q := store.QuestionAdviceJob{Question: "SIMULATED_QUESTION_CONTEXT", RuleContext: evidence, Sources: json.RawMessage(`[{"id":"owner-answer:7","text":"Owner heading"}]`)}
		a, err := w.adviseOwner(context.Background(), store.ManagementJob{Question: &q, MinimumLevel: "D2"})
		want := string(evidence)
		if evidence == nil {
			want = "no-context"
		}
		if err != nil || a.Rationale != want || a.Source != "owner-answer:7" || a.Quote != "Owner heading" || a.Context != nil || a.Session == "" {
			t.Fatal("lost question evidence", a, err)
		}
	}
}

func TestManagementFeedbackSendsCandidateEvidence(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var report struct{ Classification store.FeedbackClassification }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/management/feedback/result" || json.NewDecoder(r.Body).Decode(&report) != nil {
			t.Error("invalid feedback report")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client(), codex: os.Args[0]}
	for index, rules := range []json.RawMessage{nil, json.RawMessage(`{"requirement":{"revision":1},"rules":{"global_rules":{"revision":2}}}`)} {
		source := map[string]any{"feedback": map[string]string{"note": "SIMULATED_FEEDBACK_CONTEXT"}}
		want := "no-context"
		if rules != nil {
			source["context"] = rules
			want = string(rules)
		}
		raw, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		report.Classification = store.FeedbackClassification{}
		if err = w.classifyFeedback(context.Background(), store.FeedbackJob{ID: 1, Lease: fmt.Sprintf("00000000-0000-0000-0000-%012d", index), Source: raw}); err != nil || report.Classification.Rationale != want || report.Classification.Context != nil || report.Classification.Session == "" {
			t.Fatal("lost feedback evidence", report, err)
		}
	}
}

// The test executable doubles as a local JSONL CLI, on both Windows and Unix.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		if strings.Contains(filepath.Base(os.Args[0]), "fake-review-claude") {
			fmt.Println("1.2.3 (Claude Code)")
			os.Exit(0)
		}
		fmt.Println("codex-cli 1.2.3")
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "app-server" {
		os.Exit(testAppServerProcess())
	}
	if len(os.Args) > 1 && os.Args[1] == "-p" {
		if slices.Contains(os.Args, "--json-schema") {
			os.Exit(testReviewProcess())
		}
		os.Exit(testClaudeProcess())
	}
	if len(os.Args) <= 1 || os.Args[1] != "-a" {
		os.Exit(m.Run())
	}
	body, err := io.ReadAll(os.Stdin)
	args := strings.Join(os.Args[1:], " ")

	if strings.HasPrefix(string(body), "SIMULATED_FILE_CHANGE") {
		out := json.NewEncoder(os.Stdout)
		out.Encode(map[string]any{"type": "thread.started", "thread_id": "file-change-thread"})
		item := map[string]any{"id": "change-1", "type": "file_change", "status": "in_progress", "changes": testFileChangeFixture(false), "arguments": "private-input"}
		out.Encode(map[string]any{"type": "item.started", "item": item})
		item["status"] = "completed"
		if strings.Contains(string(body), "FAILED") {
			item["status"] = "failed"
		}
		if strings.Contains(string(body), "DECLINED") {
			item["status"] = "declined"
		}
		out.Encode(map[string]any{"type": "item.completed", "item": item})
		out.Encode(map[string]any{"type": "turn.completed"})
		os.Exit(0)
	}
	if string(body) == "SIMULATED_PUBLIC_OUTPUT" {
		fmt.Println(`{"type":"thread.started","thread_id":"public-output-thread"}`)
		fmt.Println(`{"type":"item.started","item":{"id":"tool-1","type":"command_execution","command":"private-input"}}`)
		fmt.Println(`{"type":"item.completed","item":{"id":"tool-1","type":"command_execution","status":"completed","aggregated_output":"Tool output sample.\npassword=private-value"}}`)
		fmt.Println(`{"type":"item.completed","item":{"type":"reasoning","text":"private-thought"}}`)
		fmt.Println(`{"type":"item.completed","item":{"type":"agent_message","text":"Reading the guide.\npassword=private-value"}}`)
		fmt.Println(`{"type":"turn.completed"}`)
		os.Exit(0)
	}
	if strings.Contains(string(body), "SIMULATED_CODEX_RECOVERY") {
		const thread = "a7090935-460b-4930-8484-0f2008435a45"
		resume := slices.Contains(os.Args, "resume")
		id := thread
		if resume && strings.Contains(string(body), "WRONG_THREAD") {
			id = "046dddd8-2527-45e7-a67f-701b406412aa"
		}
		fmt.Printf(`{"type":"thread.started","thread_id":"%s"}`+"\n", id)
		if !resume {
			if os.WriteFile("progress.md", []byte("First edit.\n"), 0600) != nil {
				os.Exit(2)
			}
			if strings.Contains(string(body), "TERMINAL_FAILURE") {
				fmt.Println(`{"type":"turn.failed"}`)
				if strings.Contains(string(body), "ZERO_EXIT") {
					os.Exit(0)
				}
			}
			os.Exit(23)
		}
		b, err := os.ReadFile("progress.md")
		if err != nil || string(b) != "First edit.\n" || !slices.Contains(os.Args, thread) || !strings.Contains(args, "--sandbox workspace-write") {
			os.Exit(2)
		}
		if os.WriteFile("progress.md", append(b, []byte("Finished.\n")...), 0600) != nil {
			os.Exit(2)
		}
		fmt.Println(`{"type":"turn.completed"}`)
		os.Exit(0)
	}
	if strings.Contains(args, "--sandbox read-only") {
		if err != nil || !strings.Contains(args, "--ignore-user-config") || !strings.Contains(args, "--output-schema") || os.Getenv("FORGEOS_WORKER_TOKEN") != "" || os.Getenv("GH_TOKEN") != "" {
			os.Exit(2)
		}
		for k, arg := range os.Args {
			if arg == "--output-last-message" && k+1 < len(os.Args) {
				output := `{"objective":"Handbook","milestones":[{"name":"Docs","items":[{"key":"example","objective":"Write example","scope":"README","acceptance_criteria":"Example exists","deliverables":"Example"}]}]}`
				if strings.Contains(string(body), "SIMULATED_QUOTE_REPAIR") {
					output = `{"facts":[{"statement":"Recovered.","quote":"saved resumed"}]}`
					if strings.Contains(string(body), "previous extraction") {
						output = `{"facts":[{"statement":"Recovered.","quote":"resumed at 918"}]}`
					}
				}
				if strings.Contains(string(body), "SIMULATED_PLAN_REVIEW_CONTEXT") {
					var source struct {
						Context json.RawMessage `json:"review_context"`
					}
					_, raw, _ := strings.Cut(string(body), "Leave validation_minimum empty for this review.\n")
					if json.Unmarshal([]byte(raw), &source) != nil || strings.Contains(string(body), "private-review-lease") ||
						(source.Context != nil) != strings.Contains(string(body), ordinaryRuleOrder) {
						os.Exit(5)
					}
					evidence := string(source.Context)
					if source.Context == nil {
						evidence = "no-context"
					}
					result, e := json.Marshal(store.ManagementReview{Level: "D2", Verdict: "approve", Rationale: "Rule priority checked", Evidence: evidence})
					if e != nil {
						os.Exit(6)
					}
					output = string(result)
				}
				if strings.Contains(string(body), "SIMULATED_QUESTION_CONTEXT") {
					var q store.QuestionAdviceJob
					raw := string(body)[strings.LastIndex(string(body), "\n{")+1:]
					if json.Unmarshal([]byte(raw), &q) != nil || (q.RuleContext != nil) != strings.Contains(string(body), ordinaryRuleOrder) {
						os.Exit(5)
					}
					var sources []struct{ ID, Text string }
					if json.Unmarshal(q.Sources, &sources) != nil || len(sources) != 1 {
						os.Exit(5)
					}
					evidence := string(q.RuleContext)
					if evidence == "" {
						evidence = "no-context"
					}
					result, err := json.Marshal(store.QuestionAdvice{Level: "D2", Rationale: evidence, Source: sources[0].ID, Quote: sources[0].Text})
					if err != nil {
						os.Exit(6)
					}
					output = string(result)
				}
				if strings.Contains(string(body), "SIMULATED_FEEDBACK_CONTEXT") {
					var source struct {
						Context  json.RawMessage
						Feedback struct{ Note string }
					}
					raw := string(body)[strings.LastIndex(string(body), "\n{")+1:]
					if json.Unmarshal([]byte(raw), &source) != nil || strings.Contains(string(body), "private-feedback-lease") || !strings.Contains(string(body), ordinaryRuleOrder) {
						os.Exit(5)
					}
					evidence := string(source.Context)
					if evidence == "" {
						evidence = "no-context"
					}
					result, err := json.Marshal(store.FeedbackClassification{Category: "requirement_change", Rationale: evidence, Evidence: source.Feedback.Note})
					if err != nil {
						os.Exit(6)
					}
					output = string(result)
				}
				if strings.Contains(string(body), "SIMULATED_ANALYSIS_PROCESS") {
					output = `{"facts":[{"kind":"start","value":"go run ./cmd/app","path":"README.md","quote":"go run ./cmd/app"}]}`
					if strings.Contains(string(body), "BAD_QUOTE") {
						output = `{"facts":[{"kind":"start","value":"invented","path":"README.md","quote":"invented"}]}`
					}
				}
				if strings.Contains(string(body), "SIMULATED_TEST_PLAN_PROCESS") {
					output = `{"cases":[],"clarifications":[{"criterion_id":"requirement/1","question":"Which observation is required?"}]}`
					if strings.Contains(string(body), "Act as the independent Management Agent auditing test-plan coverage") {
						output = `{"items":[{"criterion_id":"requirement/1","case_ids":[],"state":"needs_clarification","rationale":"The required observation is unspecified."}]}`
					}
				}
				if strings.Contains(string(body), "SIMULATED_CURATOR_REVIEW") {
					output = `{"level":"D1","verdict":"approve","rationale":"Recorded observation.","evidence":"resumed at 918","validation_minimum":""}`
				}
				if os.WriteFile(os.Args[k+1], []byte(output), 0600) != nil {
					os.Exit(3)
				}
				if strings.Contains(string(body), "SIMULATED_NATIVE_ROLE") {
					_, prior := os.Stat("native-started")
					resuming := strings.Contains(args, "resume a7090935-460b-4930-8484-0f2008435a45")
					missingMode := strings.Contains(string(body), "SIMULATED_NATIVE_ROLE_MISSING")
					if missingMode && resuming {
						if os.WriteFile("native-missing", []byte("missing"), 0600) != nil {
							os.Exit(8)
						}
						fmt.Fprintln(os.Stderr, "Error: thread/resume: thread/resume failed: no rollout found for thread id a7090935-460b-4930-8484-0f2008435a45 (code -32600)")
						os.Exit(1)
					}
					_, missing := os.Stat("native-missing")
					if (prior == nil) != resuming && !(missingMode && missing == nil && !resuming) {
						os.Exit(8)
					}
					if os.WriteFile("native-started", []byte("first turn"), 0600) != nil {
						os.Exit(8)
					}
					if missingMode && missing == nil {
						fmt.Println(`{"type":"thread.started","thread_id":"b7090935-460b-4930-8484-0f2008435a45"}`)
					} else {
						fmt.Println(`{"type":"thread.started","thread_id":"a7090935-460b-4930-8484-0f2008435a45"}`)
					}
				} else if strings.Contains(string(body), "SIMULATED_CURATOR_PROCESS") && strings.Contains(string(body), "previous extraction") {
					fmt.Println(`{"type":"thread.started","thread_id":"curator-repaired"}`)
				} else {
					fmt.Println(`{"type":"thread.started","thread_id":"planner-test"}`)
				}
				if strings.Contains(string(body), "SIMULATED_MANAGEMENT_PROCESS") || strings.Contains(string(body), "SIMULATED_ANALYSIS_PROCESS") || strings.Contains(string(body), "SIMULATED_ADVICE_PROCESS") || strings.Contains(string(body), "SIMULATED_FEEDBACK_PROCESS") || strings.Contains(string(body), "SIMULATED_CURATOR_PROCESS") || strings.Contains(string(body), "SIMULATED_TEST_PLAN_PROCESS") {
					fmt.Println(`{"type":"item.completed","item":{"type":"agent_message","text":"Reviewing the plan.\npassword=private-sample"}}`)
					time.Sleep(500 * time.Millisecond)
					if strings.Contains(string(body), "SIMULATED_NATIVE_ROLE_INTERRUPT") && !strings.Contains(args, "resume ") {
						os.Exit(9)
					}
					if strings.Contains(string(body), "FAIL_PROCESS") {
						fmt.Println(`{"type":"turn.failed"}`)
						os.Exit(7)
					}
				}
				fmt.Println(`{"type":"turn.completed"}`)
				os.Exit(0)
			}
		}
		os.Exit(2)
	}
	if err != nil || !strings.Contains(args, "-a never exec") || !strings.Contains(args, "--sandbox workspace-write") ||
		!strings.Contains(args, "--ignore-user-config") || os.Getenv("FORGEOS_WORKER_TOKEN") != "" || os.Getenv("GH_TOKEN") != "" {
		os.Exit(2)
	}
	if strings.Contains(string(body), "INVALID_SESSION") {
		fmt.Printf("{\"type\":\"thread.started\",\"thread_id\":\"%s\"}\n", strings.Repeat("x", 300))
		os.Exit(0)
	}
	session := "test-session"
	if strings.Contains(string(body), "CHECKPOINT_THREAD") {
		session = "a7090935-460b-4930-8484-0f2008435a45"
		continued := strings.Contains(string(body), "APPEND_EXAMPLE")
		if continued != strings.Contains(args, "resume "+session+" -") {
			os.Exit(2)
		}
	}
	if strings.Contains(string(body), `"acceptance_feedback":`) {
		session = "test-review-session"
	}
	fmt.Printf("{\"type\":\"thread.started\",\"thread_id\":\"%s\"}\n", session)
	switch {
	case strings.Contains(string(body), "SIMULATED_FAILURE"):
		fmt.Println(`{"type":"turn.failed"}`)
		os.Exit(1)
	case strings.Contains(string(body), "SIMULATED_NO_CHANGE"):
	case strings.Contains(string(body), "APPEND_EXAMPLE"):
		current, err := os.ReadFile("README.md")
		if err != nil || string(current) != "An implemented change.\n" || !strings.Contains(string(body), `"title":"Change README"`) {
			os.Exit(4)
		}
		if err := os.WriteFile("README.md", append(current, []byte("Example: start here.\n")...), 0600); err != nil {
			os.Exit(3)
		}
	default:
		if err := os.WriteFile("README.md", []byte("An implemented change.\n"), 0600); err != nil {
			os.Exit(3)
		}
	}
	fmt.Println(`{"type":"turn.completed"}`)
	os.Exit(0)
}

func TestAdapterTerminalFailure(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, adapter := range []harnessAdapter{codexCLI{bin: bin, sandbox: "workspace-write"}, claudeCLI{bin: bin}} {
		name, _ := adapter.identity()
		for _, exit := range []int{0, 23} {
			t.Run(fmt.Sprintf("%s/%d", name, exit), func(t *testing.T) {
				prompt := "SIMULATED_CODEX_RECOVERY TERMINAL_FAILURE"
				if exit == 0 {
					prompt += " ZERO_EXIT"
				}
				session, err := adapter.runTurn(context.Background(), processLog{}, t.TempDir(), prompt, "")
				var cause *exec.ExitError
				if !nativeSessionID.MatchString(session) || !errors.Is(err, errTurnFailed) || errors.Is(err, errTurnInterrupted) ||
					errors.As(err, &cause) != (exit != 0) || cause != nil && cause.ExitCode() != exit {
					t.Fatalf("session %q, failure %v, process exit %v", session, err, cause)
				}
			})
		}
	}
}

func TestCodexTurnRecovery(t *testing.T) {
	for _, input := range []string{"RESUME", "WRONG_THREAD", "TERMINAL_FAILURE"} {
		t.Run(input, func(t *testing.T) {
			w, job, base := fixture(t)
			w.codex, _ = os.Executable()
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if input == "TERMINAL_FAILURE" || r.URL.Path != "/internal/executions/12/renew" {
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				rw.WriteHeader(204)
			}))
			defer server.Close()
			w.api, w.client = server.URL, server.Client()
			job.Kind = "codex_issue"
			job.Spec = &store.IssueSpec{Issue: store.Issue{Number: 7, Title: "SIMULATED_CODEX_RECOVERY " + input}, BaseCommit: base, Validation: "docs"}
			result := w.execute(context.Background(), job)
			if result.SessionID != "a7090935-460b-4930-8484-0f2008435a45" {
				t.Fatal("lost original thread", result)
			}
			if input == "RESUME" {
				if result.ExitCode != 0 || result.Commit == "" || git(t, job.Workspace, "show", "HEAD:progress.md") != "First edit.\nFinished." {
					t.Fatal("did not finish original working copy", result)
				}
			} else if result.ExitCode == 0 || result.Commit != "" {
				t.Fatal("unexpected successful result", result)
			}
		})
	}
}

func TestCodexCommentCheckpointResumesThread(t *testing.T) {
	w, job, base := fixture(t)
	w.codex, _ = os.Executable()
	job.Kind, job.SupportsCheckpoints = "codex_issue", true
	job.Spec = &store.IssueSpec{Issue: store.Issue{Number: 7, Title: "Change README", Body: "CHECKPOINT_THREAD"}, BaseCommit: base, Validation: "docs"}
	var sessions []string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var q struct {
			Sequence int    `json:"sequence"`
			Session  string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			rw.WriteHeader(400)
			return
		}
		if q.Session != "" {
			sessions = append(sessions, q.Session)
			rw.WriteHeader(204)
			return
		}
		page := store.CheckpointPage{Checkpoint: store.Checkpoint{ExecutionID: job.ExecutionID, Sequence: q.Sequence}}
		if q.Sequence == 1 {
			page.Comments = []store.CommentInput{{EventID: 1, Body: "APPEND_EXAMPLE"}}
		}
		json.NewEncoder(rw).Encode(page)
	}))
	defer server.Close()
	w.api, w.client = server.URL, server.Client()
	result := w.execute(context.Background(), job)
	if result.ExitCode != 0 || len(sessions) != 2 || sessions[0] != sessions[1] || result.SessionID != sessions[0] {
		t.Fatal("expected both checkpoints on the original thread", sessions, result)
	}
	if git(t, job.Workspace, "show", "HEAD:README.md") != "An implemented change.\nExample: start here." {
		t.Fatal("missing checkpoint edit")
	}
}

func TestReadOnlyPlanner(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_TOKEN", "worker-test-secret")
	t.Setenv("GH_TOKEN", "github-test-secret")
	w, job, base := fixture(t)
	w.codex, _ = os.Executable()
	w.planner = true
	job.Kind = "codex_issue"
	job.Spec = &store.IssueSpec{Issue: store.Issue{Number: 42, Title: "Plan README"}, Mode: "plan", BaseCommit: base, Validation: "docs"}
	res := w.execute(context.Background(), job)
	if res.ExitCode != 0 || res.Plan == nil || res.Commit != "" || res.Validation != nil || res.SessionID != "planner-test" || res.BaseCommit != base {
		t.Fatalf("unexpected plan result: %+v", res)
	}
	if git(t, job.Workspace, "status", "--porcelain") != "" || git(t, job.Workspace, "rev-parse", "HEAD") != base {
		t.Fatal("planner modified checkout")
	}
}

func TestIssueChanges(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_TOKEN", "worker-test-secret")
	t.Setenv("GH_TOKEN", "github-test-secret")
	for _, title := range []string{"Change README", "SIMULATED_FAILURE", "SIMULATED_NO_CHANGE", "INVALID_SESSION"} {
		t.Run(title, func(t *testing.T) {
			w, job, base := fixture(t)
			w.codex, _ = os.Executable()
			job.Kind = "codex_issue"
			job.Spec = &store.IssueSpec{Issue: store.Issue{Number: 42, Title: title}, BaseCommit: base, Validation: "docs"}
			res := w.execute(context.Background(), job)
			if title == "INVALID_SESSION" {
				if res.ExitCode == 0 || res.SessionID != "" {
					t.Fatalf("invalid session ID persisted: %+v", res)
				}
				return
			}
			if res.BaseCommit != base || res.SessionID != "test-session" {
				t.Fatalf("missing issue execution evidence: %+v", res)
			}
			if title != "Change README" {
				if res.ExitCode == 0 || res.Commit != "" || res.Validation != nil {
					t.Fatalf("invalid failed or empty result: %+v", res)
				}
				return
			}
			if res.ExitCode != 0 || res.Error != "" || res.Commit == "" || res.Commit == base ||
				res.Validation == nil || res.Validation.ExitCode != 0 || res.Validation.Commit != res.Commit {
				t.Fatalf("invalid validated candidate: %+v", res)
			}
			if git(t, job.Workspace, "rev-parse", "HEAD^") != base || git(t, job.Workspace, "rev-parse", "HEAD") != res.Commit {
				t.Fatal("candidate must be exactly one local commit on the pinned base")
			}
			if _, err := os.Stat(filepath.Join(job.RepoPath, "README.md")); !os.IsNotExist(err) || git(t, job.RepoPath, "rev-parse", "HEAD") != base {
				t.Fatal("the registered repository checkout was modified")
			}
		})
	}
}

func TestCandidateKeepsPinnedParentAfterAgentCommit(t *testing.T) {
	_, job, base := fixture(t)
	ctx := context.Background()
	identity, err := checkedWorktree(ctx, job.RepoPath, job.Workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job.Workspace, "README.md"), []byte("updated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, job.Workspace, "add", "--", "README.md")
	git(t, job.Workspace, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", "agent commit")
	tree := git(t, job.Workspace, "rev-parse", "HEAD^{tree}")
	candidate, err := commitChanges(ctx, job.Workspace, base, base, 7, identity)
	if err != nil || git(t, job.Workspace, "rev-parse", candidate+"^") != base || git(t, job.Workspace, "rev-parse", candidate+"^{tree}") != tree {
		t.Fatalf("unexpected candidate after an agent commit: %s %v", candidate, err)
	}
}

func TestIssueReworkRetainsRejectedWorkspace(t *testing.T) {
	w, first, base := fixture(t)
	w.codex, _ = os.Executable()
	first.Kind = "codex_issue"
	first.Spec = &store.IssueSpec{Issue: store.Issue{Number: 7, Title: "Change README"}, BaseCommit: base, Validation: "docs"}
	ctx := context.Background()
	rejected := w.execute(ctx, first)
	if rejected.ExitCode != 0 || rejected.Commit == "" {
		t.Fatalf("first execution failed: %+v", rejected)
	}
	job := first
	job.ExecutionID++
	job.Workspace = filepath.Join(w.workspaceRoot, fmt.Sprintf("execution-%d", job.ExecutionID))
	job.Rework = &store.Review{ExecutionID: first.ExecutionID, Commit: rejected.Commit, Decision: "reject", Note: "APPEND_EXAMPLE"}
	fixed := w.execute(ctx, job)
	if fixed.ExitCode != 0 || fixed.SessionID != "test-review-session" || fixed.BaseCommit != base ||
		fixed.Commit == "" || fixed.Commit == rejected.Commit || fixed.Validation == nil ||
		fixed.Validation.Commit != fixed.Commit || fixed.Validation.ExitCode != 0 {
		t.Fatalf("revision is missing linked execution evidence: %+v", fixed)
	}
	if git(t, job.Workspace, "show", "HEAD:README.md") != "An implemented change.\nExample: start here." ||
		git(t, job.Workspace, "rev-parse", "HEAD^") != base {
		t.Fatal("revision did not include both edits as one candidate on the frozen base")
	}
	if git(t, first.Workspace, "rev-parse", "HEAD") != rejected.Commit ||
		git(t, first.Workspace, "status", "--porcelain") != "" || git(t, first.RepoPath, "rev-parse", "HEAD") != base {
		t.Fatal("rework changed an existing checkout")
	}

	job.ExecutionID++
	job.Workspace = filepath.Join(w.workspaceRoot, fmt.Sprintf("execution-%d", job.ExecutionID))
	job.Rework.Note = "SIMULATED_NO_CHANGE"
	if result := w.execute(ctx, job); result.ExitCode == 0 || result.Commit != "" || result.Validation != nil {
		t.Fatalf("unchanged rejected tree reached acceptance: %+v", result)
	}
	job.ExecutionID++
	job.Workspace = filepath.Join(w.workspaceRoot, fmt.Sprintf("execution-%d", job.ExecutionID))
	job.Rework.Commit = base
	if result := w.execute(ctx, job); result.ExitCode == 0 || result.SessionID != "" {
		t.Fatalf("unrelated rework revision reached Codex: %+v", result)
	}
}

func TestRejectsExecutableCheckoutFilters(t *testing.T) {
	w, job, base := fixture(t)
	w.codex, _ = os.Executable()
	job.Kind = "codex_issue"
	job.Spec = &store.IssueSpec{BaseCommit: base, Validation: "docs"}
	git(t, job.RepoPath, "config", "filter.example.clean", "unused")
	res := w.execute(context.Background(), job)
	if res.ExitCode == 0 || !strings.Contains(res.Error, "Git filters") || res.SessionID != "" {
		t.Fatalf("unexpected filter rejection: %+v", res)
	}
}
