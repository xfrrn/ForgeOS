package main

import (
	"context"
	"encoding/json"
	"log"

	"forgeos/internal/agents"
	"forgeos/internal/store"
)

func (w *worker) suggestIssueType(ctx context.Context, job store.ManagementJob) error {
	candidate := job.IssueClassification
	source, err := json.Marshal(candidate.Source)
	if err != nil {
		return err
	}
	const contract = `{"type":"object","additionalProperties":false,"required":["work_type","rationale","quote"],"properties":{
 "work_type":{"type":"string","enum":["development","code-review","testing","investigation","operations","clarification"]},
 "rationale":{"type":"string"},"quote":{"type":"string"}}}`
	prompt := agents.Builtin("classification") + "\n"
	var suggestion store.IssueSuggestion
	turn, cancel := managementTurn(ctx, job.LeaseUntil)
	suggestion.Session, err = w.roleJSONTurn(turn, true, store.ManagementProcessReport{IssueClassificationID: candidate.ID, Lease: job.Lease}, contract, prompt+string(source), 12<<10, &suggestion)
	cancel()
	if err != nil {
		log.Printf("Issue classification %d failed: %v", candidate.ID, err)
		return nil
	}
	profile := w.roleProfile()
	suggestion.Profile = &profile
	_, err = w.roleReceipt(ctx, "/internal/management/issue-classifications/result", map[string]any{"id": candidate.ID, "lease": job.Lease, "suggestion": suggestion})
	return err
}
