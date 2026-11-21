package main

import (
	"context"
	"errors"
	"forgeos/internal/agents"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
	"github.com/google/uuid"
)

func TestGoalPlannerDiscussion(t *testing.T) {
	t.Setenv("FORGEOS_SKILLS_DIR", t.TempDir())
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	check(data.Migrate(ctx))
	project, err := data.CreateProject(ctx, "handbook")
	check(err)
	goal, err := data.ImportIssue(ctx, project.ID, store.IssueSpec{Issue: store.Issue{ID: 1, Number: 1, RepositoryID: 9}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	check(err)
	item := store.PlanItem{Key: "usage", Objective: "Usage", Scope: "README", Deliverables: "README", Acceptance: "Example present"}
	plan := store.Plan{Objective: "Handbook", Milestones: []store.Milestone{{Name: "Docs", Items: []store.PlanItem{item}}}}
	_, err = data.SavePlanWithPolicy(ctx, goal.ID, 0, plan, "D1")
	check(err)
	_, err = data.ActivatePlan(ctx, goal.ID, 1)
	check(err)
	def, _ := agents.Lookup("planner")
	check(data.SaveAgentConfig(ctx, project.ID, def, 0, 0, &agents.Settings{Harness: "claude", Model: "sonnet", Prompt: "Review the project glossary."}))
	_, err = db.ExecContext(ctx, `INSERT INTO goal_discussion_turns(task_id,revision,client_id,item_key,message,draft,model_id,model_revision,model_name,status,answer)
 VALUES($1,1,gen_random_uuid(),'','Earlier question',false,gen_random_uuid(),gen_random_uuid(),'legacy-model','done','Preserve the prior glossary')`, goal.ID)
	check(err)
	input := store.DiscussionInput{ClientID: uuid.New(), Revision: 1, Item: "usage", Message: "Can we add an index?"}
	id, err := data.SendDiscussion(ctx, goal.ID, input)
	check(err)
	again, err := data.SendDiscussion(ctx, goal.ID, input)
	check(err)
	if again != id {
		t.Fatal("duplicate turn")
	}
	input.ClientID = uuid.New()
	if _, err = data.SendDiscussion(ctx, goal.ID, input); !errors.Is(err, store.ErrConflict) {
		t.Fatal("overlapping turn", err)
	}
	plan.Milestones[0].Items[0].Acceptance = "Example and index present"
	caps := store.WorkerCapabilities{PlannerDiscussion: true, AgentSettings: true, SupportsClaude: true, ClaudePlanning: true}
	root := t.TempDir()
	// Neither an out-of-project node nor an older node using the regular claim
	// protocol may consume the queued discussion.
	for _, probe := range []struct {
		projects     []int64
		capabilities store.WorkerCapabilities
	}{
		{[]int64{project.ID + 1}, caps},
		{[]int64{project.ID}, store.WorkerCapabilities{AgentSettings: true, SupportsClaude: true, ClaudePlanning: true}},
	} {
		job, err := data.ClaimForProjects(ctx, "other-node", root, probe.projects, probe.capabilities)
		check(err)
		if job != nil {
			t.Fatal("ineligible Worker received work", job.Kind)
		}
	}
	run := func() {
		t.Helper()
		job, err := data.ClaimForProjects(ctx, "node", root, []int64{project.ID}, caps)
		check(err)
		if job == nil || job.Discussion == nil || job.Spec.Harness != "claude" || job.Agent.Key != "planner" || job.Agent.Model != "sonnet" || job.Agent.Prompt != "Review the project glossary." || !strings.Contains(string(job.Discussion.Context), "Preserve the prior glossary") {
			t.Fatal("missing project assignment or previous reply", job)
		}
		result := store.Result{BaseCommit: job.Spec.BaseCommit, SessionID: "native-session", Harness: "claude", HarnessVersion: "1.2.3", DiscussionAnswer: "Include a glossary index."}
		if job.Discussion.Draft {
			result.Plan = &plan
			result.DiscussionAnswer = "Updated the usage acceptance criteria."
		}
		check(data.Complete(ctx, job.ExecutionID, job.LeaseToken, result))
		check(data.Complete(ctx, job.ExecutionID, job.LeaseToken, result))
	}

	run()
	// A fresh Store sees the original reply, and the next model call receives it.
	data = store.New(db)
	history, err := data.DiscussionHistory(ctx, goal.ID)
	check(err)
	if len(history) != 2 || history[1].Answer != "Include a glossary index." {
		t.Fatal(history)
	}
	input.Message = "Agreed. Keep the original example too."
	_, err = data.SendDiscussion(ctx, goal.ID, input)
	check(err)
	run()
	input.ClientID = uuid.New()
	input.Draft = true
	input.Message = ""
	_, err = data.SendDiscussion(ctx, goal.ID, input)
	check(err)
	run()
	history, err = data.DiscussionHistory(ctx, goal.ID)
	check(err)
	if len(history) != 4 || history[3].Generated != 2 {
		t.Fatal(history)
	}
	task, err := data.Task(ctx, goal.ID)
	check(err)
	if task.ActiveRevision != 1 || task.Plan.Revision != 2 || task.Status != "PLANNING" {
		t.Fatal("draft changed active plan", task.Status)
	}
	review, err := data.ClaimManagement(ctx, project.ID, true, false)
	check(err)
	if review == nil {
		t.Fatal("no review")
	}
	profile := store.StructuredCodexProfile()
	check(data.CompleteManagement(ctx, project.ID, goal.ID, 2, review.Lease, store.ManagementReview{Level: "D1", Verdict: "approve", SessionID: "manager", Rationale: "ready", Evidence: "README", Profile: &profile}))
	task, err = data.Task(ctx, goal.ID)
	check(err)
	if task.ActiveRevision != 1 || task.Plan.Review.EffectiveLevel != "D1" {
		t.Fatal("owner gate bypassed")
	}
	input.ClientID = uuid.New()
	if _, err = data.SendDiscussion(ctx, goal.ID, input); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale draft accepted", err)
	}
	task, err = data.ActivatePlan(ctx, goal.ID, 2)
	check(err)
	if task.ActiveRevision != 2 {
		t.Fatal("owner activation failed")
	}
}
