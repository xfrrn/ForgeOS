package main

import (
	"context"
	"encoding/json"

	"forgeos/internal/agents"
	"forgeos/internal/store"
)

const repositoryAnalysisSchema = `{"type":"object","additionalProperties":false,"required":["facts"],"properties":{"facts":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["kind","value","path","quote"],"properties":{"kind":{"type":"string","enum":["profile","stack","dependency","start","test"]},"value":{"type":"string"},"path":{"type":"string"},"quote":{"type":"string"}}}}}}`

func (w *worker) analyzeRepository(ctx context.Context, job store.AnalysisJob) error {
	src, err := json.Marshal(job.Input)
	if err != nil {
		return err
	}
	prompt := agents.Builtin("analysis") + "\n" + string(src)
	turn, cancel := managementTurn(ctx, job.LeaseUntil)
	var a store.RepositoryAnalysis
	session, err := w.roleJSONTurn(turn, job.CaptureProcess, store.ManagementProcessReport{AnalysisTask: job.Task, Lease: job.Lease}, repositoryAnalysisSchema, prompt, 96<<10, &a)
	cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	r := store.AnalysisResult{Task: job.Task, Lease: job.Lease}
	a.Session = session
	if job.CaptureProfile {
		profile := w.roleProfile()
		a.Profile = &profile
	}
	switch {
	case err != nil:
		r.Failure = "agent_run_failed"
	case !a.Valid(job.Input):
		r.Failure = "unverified_evidence"
	default:
		r.Analysis = &a
	}
	_, err = w.roleReceipt(ctx, "/internal/management/repository-analysis/result", r)
	return err
}
