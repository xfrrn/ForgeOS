package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"forgeos/internal/testdb"
	"github.com/google/uuid"
)

func TestProposalWorkflow(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	s := New(db)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	in := ProposalRequest{Client: uuid.New(), Lead: uuid.New(), Advisor: uuid.New(), Brief: "按名称筛选"}
	for _, model := range []uuid.UUID{in.Lead, in.Advisor} {
		_, err = db.Exec("INSERT INTO chat_models(id,model,revision) VALUES($1,$2,$3)", model, model.String(), uuid.New())
		must(err)
	}
	topic, err := s.CreateFeature(ctx, project.ID, uuid.New(), in.Lead, "筛选")
	must(err)
	start := func() (int64, error) { return s.StartFeatureProposal(ctx, project.ID, topic.ID, in, "test-head") }
	in.Materials = []ProposalMaterial{{Key: "criteria", Revision: 1}}
	if _, err := start(); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing material accepted: %v", err)
	}
	_, err = db.Exec("INSERT INTO project_materials(project_id,key,revision,title,kind,source,content,content_sha256) VALUES($1,'criteria',1,'验收','material','fixture','frozen criteria','test-digest')", project.ID)
	must(err)
	round, err := start()
	must(err)
	again, err := start()
	must(err)
	if round != again {
		t.Fatal("duplicate round")
	}
	old, err := s.ClaimChat(ctx, project.ID, true)
	must(err)
	if old != nil {
		t.Fatal("old worker received a proposal")
	}
	claim := func() *ChatJob {
		t.Helper()
		j, e := s.ClaimDiscussionJob(ctx, project.ID, true, "", true)
		must(e)
		if j == nil {
			t.Fatal("missing job")
		}
		return j
	}
	finish := func(j *ChatJob, body, failure string) error {
		return s.CompleteChat(ctx, project.ID, j.ID, j.Lease, body, failure, nil, nil)
	}
	act := func(a ProposalAction) error { return s.ActOnProposal(ctx, project.ID, topic.ID, round, a) }
	lead, advisor := claim(), claim()
	if lead.Prompt != advisor.Prompt || (!strings.Contains(lead.Prompt, in.Brief) || !strings.Contains(lead.Prompt, "frozen criteria")) {
		t.Fatal("draft input mismatch")
	}
	must(finish(lead, "lead original", ""))
	must(finish(advisor, "", "test failure"))
	job, err := s.ClaimDiscussionJob(ctx, project.ID, true, "", true)
	must(err)
	if job != nil {
		t.Fatal("comparison scheduled before both drafts")
	}
	retry := ProposalAction{Operation: "retry", Turn: advisor.ID, Client: uuid.New()}
	must(act(retry))
	must(act(retry))
	repeat := claim()
	if repeat.RequestedModel != advisor.RequestedModel || repeat.Prompt != lead.Prompt {
		t.Fatal("incorrect retry input")
	}
	must(finish(repeat, "advisor original", ""))
	must(finish(repeat, "advisor original", ""))
	comparison := claim()
	if !strings.Contains(comparison.Prompt, "lead original") || !strings.Contains(comparison.Prompt, "advisor original") {
		t.Fatal("incomplete comparison input")
	}
	must(finish(comparison, "combined v1", ""))
	ask := ProposalAction{Operation: "ask", Turn: lead.ID, Client: uuid.New(), Body: "clarify acceptance"}
	must(act(ask))
	must(act(ask))
	follow := claim()
	if !strings.Contains(follow.Prompt, "lead original") || strings.Contains(follow.Prompt, "advisor original") {
		t.Fatal("follow-up target mismatch")
	}
	must(finish(follow, "acceptance clarified", ""))
	wire := `{"title":"筛选列表","regions":[{"label":"筛选区","position":"main","elements":[{"id":"query","kind":"input","label":"搜索","value":"输入关键词"}]}]}`
	must(act(ProposalAction{Operation: "preview_proposal", Revision: 1, Client: uuid.New()}))
	must(finish(claim(), wire, ""))
	revision := ProposalAction{Operation: "revise", Revision: 1, Body: "combined v2", Element: "query", Client: uuid.New()}
	must(act(revision))
	must(act(revision))
	must(finish(claim(), wire, ""))
	approve := ProposalAction{Operation: "approve", Revision: 1}
	if !errors.Is(act(approve), ErrConflict) {
		t.Fatal("accepted stale revision")
	}
	approve.Revision = 2
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- act(approve) }()
	}
	must(<-results)
	must(<-results)
	topic, err = s.Feature(ctx, project.ID, topic.ID)
	must(err)
	if topic.Phase != "design" {
		t.Fatal(topic.Phase)
	}
	draft := claim()
	if draft.RequestedModel != lead.RequestedModel || !strings.Contains(draft.Prompt, "combined v2") {
		t.Fatal("design lost approved version")
	}
	must(finish(draft, "design first\n```forgeos-ui\n"+wire+"\n```", ""))
	review := claim()
	if review.RequestedModel != advisor.RequestedModel || !strings.Contains(review.Prompt, "design first") {
		t.Fatal("advisor did not receive design")
	}
	must(finish(review, "", "review failed"))
	retryReview := ProposalAction{Operation: "retry_design", Turn: review.ID, Revision: 2, Client: uuid.New()}
	must(act(retryReview))
	must(act(retryReview))
	review = claim()
	must(finish(review, "advisor correction", ""))
	final := claim()
	if !strings.Contains(final.Prompt, "advisor correction") || !strings.Contains(final.Prompt, "design first") {
		t.Fatal("final design missed review")
	}
	must(finish(final, "final with acceptance", ""))
	makeVisual := ProposalAction{Operation: "generate_visual", Turn: final.ID, Revision: 2, Client: uuid.New()}
	must(act(makeVisual))
	makeVisual.Client = uuid.New()
	must(act(makeVisual))
	visual := claim()
	if !strings.Contains(visual.Prompt, "final with acceptance") {
		t.Fatal("wireframe used wrong design")
	}
	must(finish(visual, "I will prepare a diagram.", ""))
	makeVisual.Client = uuid.New()
	makeVisual.Profile = in.Advisor
	must(act(makeVisual))
	visual = claim()
	if visual.RequestedModel != advisor.RequestedModel {
		t.Fatal("wireframe retry used the wrong model")
	}
	must(finish(visual, wire, ""))
	must(act(makeVisual))
	var versions int
	must(db.QueryRow("SELECT count(*) FROM project_chat_turns WHERE proposal_id=$1 AND design_wireframe IS NOT NULL", round).Scan(&versions))
	if versions != 2 {
		t.Fatal("expected both draft and final wireframes", versions)
	}
	bad := ProposalAction{Operation: "revise_design", Element: "unknown", Turn: final.ID, Revision: 2, Client: uuid.New(), Body: "test"}
	if !errors.Is(act(bad), ErrConflict) {
		t.Fatal("unknown annotation accepted")
	}
	change := ProposalAction{Operation: "revise_design", Element: "query", Turn: final.ID, Revision: 2, Client: uuid.New(), Body: "clarify wording"}
	must(act(change))
	must(act(change))
	var text string
	must(db.QueryRow("SELECT text FROM project_chat_turns WHERE proposal_id=$1 AND client_id=$2", round, change.Client).Scan(&text))
	if !strings.Contains(text, "设计 v1 · 筛选区 / 搜索 [query]") {
		t.Fatal(text)
	}
	if _, err = s.PrepareDesignDispatch(ctx, project.ID, topic.ID, round, DesignDispatch{Revision: 2, Turn: final.ID}, "team/repo", 9, "", nil); !errors.Is(err, ErrConflict) {
		t.Fatal("superseded design dispatched", err)
	}
	history, err := s.FeatureHistory(ctx, project.ID, topic.ID, 0, 30)
	must(err)
	if len(history) != 0 {
		t.Fatal("proposal leaked into discussion")
	}
	in.Client = uuid.New()
	in.Expected = 1
	in.Brief = "修改范围"
	newID, err := start()
	must(err)
	if newID == round {
		t.Fatal("next round reused old version")
	}
	newLead, newAdvisor := claim(), claim()
	go func() { results <- finish(newLead, "new lead", "") }()
	go func() { results <- finish(newAdvisor, "new advisor", "") }()
	must(<-results)
	must(<-results)
	newComparison := claim()
	if newComparison.ProposalID != newID {
		t.Fatal("misassociated comparison")
	}
	extra, err := s.ClaimDiscussionJob(ctx, project.ID, true, "", true)
	must(err)
	if extra != nil {
		t.Fatal("duplicate comparison")
	}
	if !errors.Is(act(approve), ErrConflict) {
		t.Fatal("accepted superseded round")
	}
	detail, err := s.FeatureProposal(ctx, project.ID, topic.ID, round)
	must(err)
	if !strings.Contains(string(detail), "combined v2") {
		t.Fatal("missing old revision")
	}
}
