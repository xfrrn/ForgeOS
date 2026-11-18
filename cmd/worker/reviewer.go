package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"forgeos/internal/review"
	"forgeos/internal/store"
)

func (w *worker) runReviewer(ctx context.Context) error {
	for ctx.Err() == nil {
		if err := w.replayRoleReceipts(ctx, "reviewer"); err != nil {
			return err
		}
		var job store.ReviewJob
		status, err := w.claimRole(ctx, "reviewer", "/internal/reviewer/claim?supports_role_resume=1&supports_process=1&supports_budget_deadline=1", map[string]bool{"supports_candidate_context": true}, &job)
		if status == http.StatusNoContent || temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil {
			return err
		}
		if status != http.StatusOK || job.ExecutionID < 1 || job.Lease == "" {
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
		if _, err = w.roleReceipt(ctx, "/internal/reviewer/result", out); err != nil {
			return err
		}
	}
	return nil
}

func (w *worker) reviewJob(ctx context.Context, job store.ReviewJob) (review.Report, error) {
	input := candidateReview{Repo: job.Repo, relative: job.RepoRelativePath, Base: job.Base, Commit: job.Commit, Requirement: job.Requirement, CandidateContext: job.CandidateContext}
	target := store.ManagementProcessReport{ReviewerExecution: job.ExecutionID, Lease: job.Lease}
	var report review.Report
	if _, found, err := w.completedRoleTurn(target, &report); found || err != nil {
		return report, err
	}
	if !job.CaptureProcess {
		return reviewCandidate(ctx, input, processLog{beforeStart: w.roleStartHook("reviewer", job.Lease, 0)})
	}
	version, err := w.roleHarnessVersion("claude")
	if err != nil {
		return review.Report{}, err
	}
	_, err = w.roleProcessTurn(ctx, target, "claude", version, &report, func(ctx context.Context, p processLog) (string, error) {
		var e error
		report, e = reviewCandidate(ctx, input, p)
		return report.Session, e
	})
	return report, err
}
