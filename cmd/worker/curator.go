package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"forgeos/internal/agents"
	"forgeos/internal/store"
)

const extractionSchema = agents.ExtractionSchema

func (w *worker) runCurator(ctx context.Context) error {
	for ctx.Err() == nil {
		if err := w.replayRoleReceipts(ctx, "curator"); err != nil {
			return err
		}
		var job store.CurationJob
		status, err := w.claimRole(ctx, "curator", "/internal/curator/claim?agent_settings=1&supports_role_resume=1&supports_process=1&supports_budget_deadline=1", struct{}{}, &job)
		if status == 204 || temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("curator claim rejected (HTTP %d)", status)
		}
		if err = w.curate(ctx, job, false); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (w *worker) curate(ctx context.Context, job store.CurationJob, review bool) error {
	deadline := time.Now().Add(5 * time.Minute)
	if job.LeaseUntil != nil && job.LeaseUntil.Before(deadline) {
		deadline = *job.LeaseUntil
	}
	turn, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	var err error
	var path string
	report := map[string]any{"id": job.ID, "lease": job.Lease}
	var profile *store.AgentProfile
	if job.CaptureProfile {
		p := w.roleProfile()
		profile = &p
	}
	if review {
		doc, e := job.ReviewInput()
		if e != nil {
			return e
		}
		prompt := agents.Builtin("knowledge-review") + "\nSource:\n" + string(job.Material) + "\nProposal:\n" + string(doc)
		var r store.ManagementReview
		r.SessionID, err = w.roleJSONTurn(turn, job.CaptureProcess, store.ManagementProcessReport{CurationID: job.ID, CurationRole: "management", Turn: 1, Lease: job.Lease}, reviewSchema, prompt, 16<<10, &r)
		r.Profile = profile
		report["review"] = r
		path = "/internal/management/curation/result"
	} else {
		var out store.Extraction
		out, err = w.extractFacts(turn, job)
		out.Profile = profile
		report["extraction"] = out
		path = "/internal/curator/result"
	}
	if err != nil {
		log.Printf("curation %d model turn failed: %v", job.ID, err)
		return nil
	}
	_, err = w.roleReceipt(ctx, path, report)
	return err
}

func (w *worker) extractFacts(ctx context.Context, job store.CurationJob) (store.Extraction, error) {
	material := job.Material
	var source struct{ Content string }
	if err := json.Unmarshal(material, &source); err != nil {
		return store.Extraction{}, err
	}
	prompt := agents.Builtin("curator") + "\n" + string(material)
	for attempt := 1; attempt <= 2; attempt++ {
		var out store.Extraction
		session, err := w.roleJSONTurn(ctx, job.CaptureProcess, store.ManagementProcessReport{CurationID: job.ID, CurationRole: "curator", Turn: attempt, Lease: job.Lease}, extractionSchema, prompt, 64<<10, &out)
		if err != nil {
			return out, err
		}
		out.Session = session
		if out.HasSourceQuotes(source.Content) {
			return out, nil
		}
		prompt = "The previous extraction did not supply a contiguous source quote for every finding. Select one exact uninterrupted phrase from content for each finding, without combining separated passages. Omit a finding if unsupported.\n" + prompt
	}
	return store.Extraction{}, fmt.Errorf("extraction did not supply exact source quotes")
}
