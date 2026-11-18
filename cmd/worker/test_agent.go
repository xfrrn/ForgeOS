package main

import (
	"context"
	"encoding/json"
	"fmt"
	"forgeos/internal/store"
	"log"
	"time"
)

func (w *worker) runTestAgent(ctx context.Context) error {
	for ctx.Err() == nil {
		if err := w.replayRoleReceipts(ctx, "test-agent"); err != nil {
			return err
		}
		var job store.TestPlanJob
		status, err := w.post(ctx, "/internal/test-agent/claim?supports_process=1&supports_candidate_context=1&supports_budget_deadline=1", struct{}{}, &job)
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
	turn := ctx
	if job.LeaseUntil != nil {
		var stop context.CancelFunc
		turn, stop = context.WithDeadline(ctx, *job.LeaseUntil)
		defer stop()
	}
	role := "test"
	if management {
		role = "management"
	}
	run := func(ctx context.Context, contract, prompt string, input, output any) (string, error) {
		body, e := json.Marshal(input)
		if e != nil {
			return "", e
		}
		ctx, stop := context.WithTimeout(ctx, 5*time.Minute)
		defer stop()
		return w.roleJSONTurn(ctx, job.CaptureProcess, store.ManagementProcessReport{TestPlanID: job.ID, TestPlanRole: role, Lease: job.Lease}, contract, prompt+string(body), 256<<10, output)
	}
	if management {
		path = "/internal/management/test-plans/result"
		if job.Plan == nil {
			return fmt.Errorf("missing plan for review %d", job.ID)
		}
		report, e := coverageFromSource(turn, job.Source, *job.Plan, run)
		err = e
		if err == nil {
			out.Coverage = &report
		}
	} else {
		plan, e := planFromSource(turn, job.Source, run)
		err = e
		if err == nil {
			out.Plan = &plan
		}
	}
	if err != nil {
		log.Printf("test plan job %d: %v", job.ID, err)
		out.Failed = true
	} else if job.CaptureProfile {
		profile := store.StructuredCodexProfile()
		out.Profile = &profile
	}
	_, err = w.roleReceipt(ctx, path, out)
	return err
}
