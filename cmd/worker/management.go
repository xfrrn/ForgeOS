package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"forgeos/internal/agents"
	"forgeos/internal/store"
)

const reviewSchema = agents.ReviewSchema

func (w *worker) reviewManagement(ctx context.Context, job store.ManagementJob) (store.ManagementReview, error) {
	var review store.ManagementReview
	var source any = struct {
		Spec    store.IssueSpec `json:"frozen_request"`
		Plan    store.Plan      `json:"plan"`
		Context json.RawMessage `json:"review_context,omitempty"`
	}{job.Spec, job.Plan, job.ReviewContext}
	prompt := agents.Builtin("plan-review") + " The policy floor is " + job.MinimumLevel + ". Give your rationale and evidence in the issue's language.\n"
	if job.ReviewContext != nil {
		prompt += ordinaryRuleOrder + "Use review_context as the frozen rule evidence for this review. Apply each decision only to its listed plan_keys and stated validity. Cite rule revisions or decision ids when they affect your verdict. Missing or inapplicable references do not authorize a choice.\n"
	}
	if job.Share != nil {
		source = job.Share
		prompt = agents.Builtin("share") + "\n"
	}
	doc, err := json.Marshal(source)
	if err != nil {
		return review, err
	}
	if job.Spec.Policy != nil && job.Share == nil {
		prompt += "Choose a validation_minimum for future work items if the evidence calls for stronger checks: V2 unit/build, V3 integration, V4 security and critical regression. Cite the reason; otherwise leave it empty. Decision level D1-D3 is separate.\n"
	} else {
		prompt += "Leave validation_minimum empty for this review.\n"
	}
	session, err := w.managementReviewTurn(ctx, job, prompt+string(doc), &review)
	review.SessionID = session
	if job.CaptureProfile {
		p := w.roleProfile()
		review.Profile = &p
	}
	return review, err
}

func readOnlyJSONTurn(ctx context.Context, bin, contract, prompt string, limit int64, dest any) (string, error) {
	return recordedJSONTurn(ctx, bin, contract, prompt, limit, dest, processLog{})
}

func recordedJSONTurn(ctx context.Context, bin, contract, prompt string, limit int64, dest any, process processLog) (string, error) {
	var workspace string
	var err error
	if process.roleNative != nil {
		workspace = process.roleNative.Workspace
		prompt, contract, err = process.freezeRoleInput(prompt, contract)
		if err != nil {
			return "", err
		}
	} else {
		workspace, err = os.MkdirTemp("", "forgeos-readonly-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(workspace)
	}
	schema, output := filepath.Join(workspace, "schema.json"), filepath.Join(workspace, "output.json")
	if err = os.WriteFile(schema, []byte(contract), 0600); err != nil {
		return "", err
	}
	options := []string{"--skip-git-repo-check", "--output-schema", schema, "--output-last-message", output}
	profile := store.StructuredCodexProfile()
	adapter := codexCLI{bin: bin, sandbox: profile.Sandbox, disabledFeatures: profile.DisabledFeatures, options: options}
	resume := ""
	if process.roleNative != nil {
		resume = process.roleNative.Resume
	}
	session, err := adapter.runTurn(ctx, process, workspace, prompt, resume)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(output)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return "", errors.New("invalid structured output file")
	}
	doc, err := os.ReadFile(output)
	if err != nil {
		return "", err
	}
	if err = json.Unmarshal(doc, dest); err != nil {
		return "", errors.New("invalid structured output JSON")
	}
	return session, nil
}

func managementTurn(ctx context.Context, until *time.Time) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(5 * time.Minute)
	if until != nil && until.Before(deadline) {
		deadline = *until
	}
	return context.WithDeadline(ctx, deadline)
}

func (w *worker) runManagement(ctx context.Context) error {
	for ctx.Err() == nil {
		if err := w.replayRoleReceipts(ctx, "management"); err != nil {
			return err
		}
		var job store.ManagementJob
		status, err := w.claimRole(ctx, "management", "/internal/management/claim?agent_settings=1&supports_issue_classification=1&supports_role_resume=1&supports_test_plan_process=1&supports_curation_process=1&supports_feedback_process=1&supports_question_process=1&supports_analysis_process=1&supports_share_review_process=1&supports_plan_review_process=1&supports_feedback_context=1&supports_question_context=1&supports_plan_review_context=1&supports_question_deadline=1&supports_feedback_deadline=1&supports_analysis_deadline=1&supports_share_review_deadline=1&supports_plan_review_deadline=1&supports_curation=1&curation_acp=1.0&supports_candidate_context=1&supports_question_advice=1&supports_decision_lookups=1&supports_budget_deadline=1", map[string]bool{"supports_shares": true, "supports_test_plans": true, "supports_feedback": true, "supports_repository_analysis": true}, &job)
		if err != nil && !temporary(status) {
			return err
		}
		if status == http.StatusNoContent || temporary(status) {
			wait(ctx)
			continue
		}
		if status != http.StatusOK {
			return fmt.Errorf("management claim failed (HTTP %d)", status)
		}
		if err != nil {
			return err
		}
		if job.IssueClassification != nil {
			if err = w.suggestIssueType(ctx, job); err != nil {
				return err
			}
			continue
		}
		if job.Curation != nil {
			if err = w.curate(ctx, *job.Curation, true); err != nil {
				return err
			}
			continue
		}
		if job.TestPlan != nil {
			if err = w.finishTestJob(ctx, *job.TestPlan, true); err != nil {
				return err
			}
			continue
		}
		if job.RepositoryAnalysis != nil {
			if err = w.analyzeRepository(ctx, *job.RepositoryAnalysis); err != nil {
				return err
			}
			continue
		}
		if job.Feedback != nil {
			if err = w.classifyFeedback(ctx, *job.Feedback); err != nil {
				return err
			}
			continue
		}
		if job.Question != nil {
			turn, cancel := managementTurn(ctx, job.LeaseUntil)
			advice, err := w.adviseOwner(turn, job)
			cancel()
			if err != nil {
				log.Printf("management question %d review failed: %v", job.Question.ID, err)
				continue
			}
			if job.CaptureProfile {
				profile := w.roleProfile()
				advice.Profile = &profile
			}
			status, err = w.roleReceipt(ctx, "/internal/management/questions/result", map[string]any{"id": job.Question.ID, "lease": job.Lease, "advice": advice})
			if err != nil {
				return err
			}
			log.Printf("management question %d (HTTP %d)", job.Question.ID, status)
			continue
		}
		turn, cancel := managementTurn(ctx, job.LeaseUntil)
		review, err := w.reviewManagement(turn, job)
		cancel()
		if err != nil {
			if job.Share != nil {
				log.Printf("management review for share %d failed: %v", job.Share.ID, err)
			} else {
				log.Printf("management review for task %d revision %d failed: %v", job.TaskID, job.Revision, err)
			}
			continue
		}
		report := struct {
			ShareID  int64                  `json:"share_id,omitempty"`
			TaskID   int64                  `json:"task_id"`
			Revision int                    `json:"revision"`
			Lease    string                 `json:"lease"`
			Review   store.ManagementReview `json:"review"`
		}{TaskID: job.TaskID, Revision: job.Revision, Lease: job.Lease, Review: review}
		if job.Share != nil {
			report.ShareID = job.Share.ID
		}
		status, err = w.roleReceipt(ctx, "/internal/management/result", report)
		if err != nil {
			return err
		}
		if job.Share != nil {
			log.Printf("management review share %d (HTTP %d)", job.Share.ID, status)
		} else {
			log.Printf("management review task %d revision %d (HTTP %d)", job.TaskID, job.Revision, status)
		}
	}
	return nil
}

func (w *worker) adviseOwner(ctx context.Context, job store.ManagementJob) (store.QuestionAdvice, error) {
	question, floor := *job.Question, job.MinimumLevel
	var advice store.QuestionAdvice
	raw, err := json.Marshal(question)
	if err != nil {
		return advice, err
	}
	const schema = `{"type":"object","additionalProperties":false,"required":["level","rationale","suggestion","source","quote"],
		"properties":{"level":{"type":"string","enum":["D1","D2","D3"]},"rationale":{"type":"string"},"suggestion":{"type":"string"},
		"source":{"type":"string"},"quote":{"type":"string"}}}`
	prompt := agents.Builtin("question") + " The policy floor is " + floor + ". Do not claim the Owner has approved anything.\n"
	if question.RuleContext != nil {
		prompt += ordinaryRuleOrder + "The rule_context preserves this execution's evidence, including already processed Owner instructions, earlier answers and acceptance feedback. Respect each decision's stated scope. It does not contain unprocessed commands or newer global rules. Use the source ids provided in sources to cite these records.\n"
	}
	advice.Session, err = w.roleJSONTurn(ctx, job.CaptureProcess, store.ManagementProcessReport{QuestionID: question.ID, Lease: job.Lease}, schema, prompt+string(raw), 16<<10, &advice)
	return advice, err
}
