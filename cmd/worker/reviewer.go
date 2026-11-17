package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"forgeos/internal/store"
)

func (w *worker) runReviewer(ctx context.Context) error {
	for ctx.Err() == nil {
		var job store.ReviewJob
		status, err := w.post(ctx, "/internal/reviewer/claim", map[string]bool{"supports_candidate_context": true}, &job)
		if status == http.StatusNoContent || temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil || status != http.StatusOK || job.ExecutionID < 1 || job.Lease == "" {
			return fmt.Errorf("invalid reviewer claim (HTTP %d)", status)
		}
		report, err := reviewCandidate(ctx, candidateReview{Repo: job.Repo, relative: job.RepoRelativePath, Base: job.Base, Commit: job.Commit, Requirement: job.Requirement, CandidateContext: job.CandidateContext})
		if ctx.Err() != nil {
			return nil
		}
		out := map[string]any{"execution_id": job.ExecutionID, "lease": job.Lease}
		if err != nil {
			log.Printf("candidate review %d failed: %v", job.ExecutionID, err)
			out["failed"] = true
		} else {
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
