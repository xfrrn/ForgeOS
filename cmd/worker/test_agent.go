package main

import (
	"context"
	"fmt"
	"forgeos/internal/store"
	"log"
	"net/http"
)

func (w *worker) runTestAgent(ctx context.Context) error {
	for ctx.Err() == nil {
		var job store.TestPlanJob
		status, err := w.post(ctx, "/internal/test-agent/claim?supports_candidate_context=1", struct{}{}, &job)
		if status == 204 || temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil || status != 200 || job.Lease == "" {
			return fmt.Errorf("invalid Test Agent claim (HTTP %d)", status)
		}
		if err = w.finishTestJob(ctx, job, false); err != nil {
			return err
		}
	}
	return nil
}

func (w *worker) finishTestJob(ctx context.Context, job store.TestPlanJob, management bool) error {
	path := "/internal/test-agent/result"
	out := store.TestPlanOutcome{ID: job.ID, Lease: job.Lease}
	var err error
	if management {
		path = "/internal/management/test-plans/result"
		if job.Plan == nil {
			return fmt.Errorf("missing plan for review %d", job.ID)
		}
		report, e := coverageFromSource(ctx, job.Source, *job.Plan)
		err = e
		if err == nil {
			out.Coverage = &report
		}
	} else {
		plan, e := planFromSource(ctx, job.Source)
		err = e
		if err == nil {
			out.Plan = &plan
		}
	}
	if err != nil {
		log.Printf("test plan job %d: %v", job.ID, err)
		out.Failed = true
	}
	for ctx.Err() == nil {
		status, _ := w.post(ctx, path, out, nil)
		if status == http.StatusNoContent || status == http.StatusConflict {
			return nil
		}
		if !temporary(status) {
			return fmt.Errorf("test plan job %d rejected (HTTP %d)", job.ID, status)
		}
		wait(ctx)
	}
	return nil
}
