package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"forgeos/internal/agents"
	"forgeos/internal/localrepo"
	"forgeos/internal/review"
	"forgeos/internal/store"
)

//go:embed code_review_schema.json
var codeReviewSchema string

type candidateReview struct {
	CandidateContext json.RawMessage `json:"candidate_context,omitempty"`
	relative         string
	Repo             string `json:"repo"`
	Base             string `json:"base_commit"`
	Commit           string `json:"commit"`
	Requirement      string `json:"requirement"`
}

func reviewCandidate(ctx context.Context, input candidateReview, process processLog) (review.Report, error) {
	var report review.Report
	if os.Geteuid() == 0 || strings.TrimSpace(input.Requirement) == "" || len(input.Requirement) > 64<<10 {
		return report, errors.New("review requires a non-root identity and a bounded requirement")
	}
	repo, err := localrepo.ResolveAssigned(ctx, os.Getenv("FORGEOS_REPO_ROOT"), input.Repo, input.relative)
	if err != nil {
		return report, err
	}
	snapshot, err := review.Read(ctx, repo, input.Base, input.Commit)
	if err != nil {
		return report, err
	}
	report, err = reviewSnapshot(ctx, snapshot, input, process)
	if err != nil {
		return report, err
	}
	err = snapshot.Verify(&report)
	return report, err
}

func reviewSnapshot(ctx context.Context, snapshot review.Snapshot, input candidateReview, process processLog) (review.Report, error) {
	var report review.Report
	agent := runtimeAgent(ctx)
	payload, err := json.Marshal(struct {
		CandidateContext json.RawMessage `json:"candidate_context,omitempty"`
		Requirement      string          `json:"requirement"`
		Snapshot         review.Snapshot `json:"snapshot"`
	}{input.CandidateContext, input.Requirement, snapshot})
	if err != nil {
		return report, err
	}
	prompt := agents.Builtin("reviewer") + "\n"
	if agent != nil {
		prompt = agent.Apply(prompt)
	}
	profile := review.ClaudeProfile()
	var output struct {
		Summary  string           `json:"summary"`
		Findings []review.Finding `json:"findings"`
	}
	var session string
	if agent != nil && agent.Harness == "codex" {
		profile = store.StructuredCodexProfile()
		session, err = recordedJSONTurn(ctx, agent.worker.codex, codeReviewSchema, prompt+string(payload), 1<<20, &output, process)
	} else {
		session, err = claudeRoleJSON(ctx, codeReviewSchema, prompt+string(payload), 1<<20, &output, process)
	}
	if err != nil {
		return report, err
	}
	return review.Report{Base: snapshot.Base, Commit: snapshot.Commit, Session: session, Summary: output.Summary, Findings: output.Findings, Profile: &profile}, nil
}

func runCandidateReview(ctx context.Context, in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(io.LimitReader(in, (128<<10)+1))
	if err != nil || len(raw) > 128<<10 {
		return errors.New("review request too large or unreadable")
	}
	var input candidateReview
	if err = json.Unmarshal(raw, &input); err != nil {
		return errors.New("invalid candidate review request")
	}
	report, err := reviewCandidate(ctx, input, processLog{})
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(report)
}
