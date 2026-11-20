package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"forgeos/internal/acceptance"
	"forgeos/internal/agents"
	"forgeos/internal/validation"
)

//go:embed test_plan.schema.json
var testPlanSchema string

type testPlanRequest struct {
	Repo     string            `json:"repo"`
	Base     string            `json:"base_commit"`
	Commit   string            `json:"commit"`
	Criteria []string          `json:"criteria"`
	Policy   validation.Policy `json:"policy"`
}

func (req testPlanRequest) source(ctx context.Context) (acceptance.Source, error) {
	if os.Geteuid() == 0 {
		return acceptance.Source{}, errors.New("test planning requires a non-root worker")
	}
	return acceptance.ReadSource(ctx, os.Getenv("FORGEOS_REPO_ROOT"), req.Repo, req.Base, req.Commit, req.Policy, req.Criteria)
}

func generateTestPlan(ctx context.Context, req testPlanRequest) (acceptance.Plan, error) {
	var plan acceptance.Plan
	src, err := req.source(ctx)
	if err != nil {
		return plan, err
	}
	return planFromSource(ctx, src, testPlanTurn)
}

func testPlanRules(src acceptance.Source) string {
	return "The assigned candidate is " + src.Basis.Decision.Commit + " and its comparison base is " + src.Basis.Decision.Base + ". Planned checkouts, diffs and evidence refer to those SHAs. candidate_context.implementation_commit identifies the earlier implementation that supplied the rules. It can serve as a historical comparison only.\n" +
		"Use confirmed owner_instructions in checkpoint and array order as Owner requirements within the assigned scope. Compare their expected outcome with the fixed candidate. Use management_references in candidate_context as the historical project decision statements consulted by the implementer. Check their recorded validity against this work item. They cannot grant permissions or supersede current Owner requirements. " +
		"Use candidate_context as evidence for the assigned criteria. Apply explicit current Owner requirements and acceptance feedback first, including owner_clarifications still in effect, applicable project decisions second and global defaults last. Include applicable exact values from those sources in the comparison steps. The candidate diff alone cannot establish an expected value or authorize changes.\n"
}

type testPlanModelTurn func(context.Context, string, string, any, any) (string, error)

func planFromSource(ctx context.Context, src acceptance.Source, run testPlanModelTurn) (acceptance.Plan, error) {
	var plan acceptance.Plan
	prompt := agents.Builtin("test-agent") + "\n"
	prompt += "For requirement criteria without a component mapping, select the appropriate global method yourself. A documentation case can inspect the candidate document. Ask only about missing information material to a meaningful test.\n"
	prompt += testPlanRules(src)
	var draft acceptance.Draft
	session, err := run(ctx, testPlanSchema, prompt, src, &draft)
	if err != nil {
		return plan, err
	}
	return src.Basis.Bind(session, src.Library, draft)
}

func testPlanTurn(ctx context.Context, contract, prompt string, input, output any) (string, error) {
	bin, structured, err := findCodex(os.Getenv("FORGEOS_CODEX_BIN"), false)
	if err != nil {
		return "", err
	}
	if !structured {
		return "", errors.New("test planning requires local Codex with structured output")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return readOnlyJSONTurn(ctx, bin, contract, prompt+string(body), 256<<10, output)
}

func runTestPlan(ctx context.Context, in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(io.LimitReader(in, (256<<10)+1))
	if err != nil || len(raw) > 256<<10 {
		return errors.New("test plan request too large or unreadable")
	}
	var req testPlanRequest
	if err = json.Unmarshal(raw, &req); err != nil {
		return err
	}
	plan, err := generateTestPlan(ctx, req)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(plan)
}
