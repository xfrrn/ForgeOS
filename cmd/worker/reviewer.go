package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"forgeos/internal/review"
	"forgeos/internal/store"
)

func (w *worker) runReviewer(ctx context.Context) error {
	for ctx.Err() == nil {
		var job store.ReviewJob
		status, err := w.post(ctx, "/internal/reviewer/claim?supports_process=1&supports_budget_deadline=1", map[string]bool{"supports_candidate_context": true}, &job)
		if status == http.StatusNoContent || temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil || status != http.StatusOK || job.ExecutionID < 1 || job.Lease == "" {
			return fmt.Errorf("invalid reviewer claim (HTTP %d)", status)
		}
		turn := ctx
		var stop context.CancelFunc
		if job.LeaseUntil != nil {
			turn, stop = context.WithDeadline(ctx, *job.LeaseUntil)
		}
		report, err := w.reviewJob(turn, job)
		if stop != nil {
			stop()
		}
		if ctx.Err() != nil {
			return nil
		}
		out := map[string]any{"execution_id": job.ExecutionID, "lease": job.Lease}
		if err != nil {
			log.Printf("candidate review %d failed: %v", job.ExecutionID, err)
			out["failed"] = true
		} else {
			if !job.CaptureProfile {
				report.Profile = nil
			}
			out["report"] = report
		}
		for ctx.Err() == nil {
			status, _ = w.post(ctx, "/internal/reviewer/result", out, nil)
			if status == http.StatusNoContent || status == http.StatusConflict {
				break
			}
			if !temporary(status) {
				return fmt.Errorf("review %d result rejected (HTTP %d)", job.ExecutionID, status)
			}
			wait(ctx)
		}
	}
	return nil
}

func (w *worker) reviewJob(ctx context.Context, job store.ReviewJob) (review.Report, error) {
	input := candidateReview{Repo: job.Repo, relative: job.RepoRelativePath, Base: job.Base, Commit: job.Commit, Requirement: job.Requirement, CandidateContext: job.CandidateContext}
	if !job.CaptureProcess {
		return reviewCandidate(ctx, input, processLog{})
	}
	name := os.Getenv("FORGEOS_CLAUDE_BIN")
	if name == "" {
		name = "claude"
	}
	call, stop := context.WithTimeout(ctx, 10*time.Second)
	cmd := exec.CommandContext(call, name, "--version")
	cmd.Env = harnessEnv()
	raw, err := cmd.Output()
	stop()
	version := claudeVersion.FindStringSubmatch(strings.TrimSpace(string(raw)))
	if err != nil || len(version) != 2 {
		return review.Report{}, fmt.Errorf("cannot identify the Claude Code reviewer version")
	}
	var report review.Report
	_, err = w.roleProcessTurn(ctx, store.ManagementProcessReport{ReviewerExecution: job.ExecutionID, Lease: job.Lease}, "claude", version[1], func(ctx context.Context, p processLog) (string, error) {
		var e error
		report, e = reviewCandidate(ctx, input, p)
		return report.Session, e
	})
	return report, err
}
