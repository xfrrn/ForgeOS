package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestPlanningMilestoneRetrospective(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	other, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	work, err := s.CreateTask(ctx, p.ID)
	must(err)
	foreign, err := s.CreateTask(ctx, other.ID)
	must(err)
	var rev int64
	edit := func(e PlanningEdit) {
		t.Helper()
		e.Revision = &rev
		must(s.EditPlanning(ctx, p.ID, e, "org/repo", 42))
		rev++
	}
	denied := func(e PlanningEdit) {
		t.Helper()
		e.Revision = &rev
		if err := s.EditPlanning(ctx, p.ID, e, "", 0); !errors.Is(err, ErrConflict) {
			t.Fatalf("expected conflict for %s: %v", e.Action, err)
		}
	}
	type row struct {
		ID         int64
		Title      string
		StartDate  *string `json:"start_date"`
		EndDate    *string `json:"end_date"`
		Deliveries []struct {
			StartDate *string `json:"start_date"`
			EndDate   *string `json:"end_date"`
			ID        int64
			Tasks     []struct{ ID int64 }
		}
	}
	read := func() (d struct {
		Milestones []row
		Retros     []struct {
			ID       int64
			Snapshot []row
		}
		Improvements []row
		Excerpts     []struct{ Entry int64 }
		Drafts       []row
	}) {
		t.Helper()
		raw, err := s.ProjectPlanning(ctx, p.ID)
		must(err)
		must(json.Unmarshal(raw, &d))
		return
	}
	edit(PlanningEdit{Action: "overview", Body: "# Roadmap"})
	start, end, empty, bad := "2026-11-16", "2026-11-20", "", "2026-02-30"
	denied(PlanningEdit{Action: "milestone", Title: "Invalid", StartDate: &end, EndDate: &start})
	denied(PlanningEdit{Action: "milestone", Title: "Invalid", StartDate: &bad, EndDate: &empty})
	edit(PlanningEdit{Action: "milestone", Title: "First", StartDate: &start, EndDate: &end})
	edit(PlanningEdit{Action: "milestone", Title: "Next"})
	ms := read().Milestones
	a, b := ms[0].ID, ms[1].ID
	if ms[1].StartDate != nil || ms[1].EndDate != nil {
		t.Fatal("invented dates")
	}
	edit(PlanningEdit{Action: "milestone", ID: a, Title: "First edited by legacy client"})
	dates := read().Milestones[0]
	if dates.StartDate == nil || *dates.StartDate != start || dates.EndDate == nil || *dates.EndDate != end {
		t.Fatal("lost stored dates")
	}
	edit(PlanningEdit{Action: "delivery", Title: "Small release", Target: a})
	firstItem := read().Milestones[0].Deliveries[0]
	if firstItem.StartDate != nil || len(firstItem.Tasks) != 0 || len(read().Drafts) != 0 {
		t.Fatal("name-only item must be unscheduled and undispatched")
	}
	denied(PlanningEdit{Action: "delivery", ID: firstItem.ID, Title: "Bad dates", StartDate: &end, EndDate: &start})
	edit(PlanningEdit{Action: "delivery", ID: firstItem.ID, Title: "Small release", StartDate: &start, EndDate: &end})
	edit(PlanningEdit{Action: "delivery", ID: firstItem.ID, Title: "Small release", Body: "Smoke check"})
	dated := read().Milestones[0].Deliveries[0]
	if dated.StartDate == nil || *dated.StartDate != start || dated.EndDate == nil || *dated.EndDate != end {
		t.Fatal("lost planned task dates")
	}
	d := read().Milestones[0].Deliveries[0].ID
	denied(PlanningEdit{Action: "link", ID: d, Target: foreign.ID})
	edit(PlanningEdit{Action: "link", ID: d, Target: work.ID})
	edit(PlanningEdit{Action: "draft", ID: d, Title: "Release", Body: "Smoke check", Flag: true})
	if len(read().Drafts) != 1 {
		t.Fatal("missing dispatch draft")
	}
	edit(PlanningEdit{Action: "retro", Title: "Release review", Target: a, Body: "One observation. Reusable lesson."})
	r := read().Retros[0]
	if len(r.Snapshot) != 1 || r.Snapshot[0].Deliveries[0].Tasks[0].ID != work.ID {
		t.Fatal(r)
	}
	edit(PlanningEdit{Action: "delivery", ID: d, Title: "Small release", StartDate: &empty, EndDate: &empty})
	if item := read().Milestones[0].Deliveries[0]; item.EndDate != nil || item.StartDate != nil || len(item.Tasks) != 1 {
		t.Fatal("clearing dates changed task linkage")
	}
	if frozen := read().Retros[0].Snapshot[0].Deliveries[0].EndDate; frozen == nil || *frozen != end {
		t.Fatal("lost original item schedule in retrospective")
	}
	edit(PlanningEdit{Action: "milestone", ID: a, Title: "First", StartDate: &empty, EndDate: &empty})
	if m := read().Milestones[0]; m.StartDate != nil || m.EndDate != nil {
		t.Fatal("dates not cleared")
	}
	if saved := read().Retros[0].Snapshot[0]; saved.EndDate == nil || *saved.EndDate != end {
		t.Fatal("changed historical dates")
	}
	edit(PlanningEdit{Action: "improvement", Target: r.ID, Title: "Automate smoke check"})
	i := read().Improvements[0].ID
	denied(PlanningEdit{Action: "carry", ID: i, Target: a})
	edit(PlanningEdit{Action: "carry", ID: i, Target: b})
	denied(PlanningEdit{Action: "carry", ID: i, Target: b})
	denied(PlanningEdit{Action: "excerpt", ID: r.ID, Title: "Lesson", Body: "invented"})
	edit(PlanningEdit{Action: "excerpt", ID: r.ID, Title: "Lesson", Body: "Reusable lesson."})
	hits, err := s.SearchExperience(ctx, p.ID, "Reusable")
	must(err)
	if string(hits) == "[]" {
		t.Fatal("excerpt missing from search")
	}
	hits, err = s.SearchExperience(ctx, p.ID, "observation")
	must(err)
	if string(hits) != "[]" {
		t.Fatal("unselected text became knowledge")
	}
	state := read()
	result, err := s.ExperienceEntry(ctx, p.ID, state.Excerpts[0].Entry)
	must(err)
	var entry struct{ Status, Statement string }
	must(json.Unmarshal(result, &entry))
	if entry.Status != "current" || entry.Statement != "Reusable lesson." {
		t.Fatal(entry)
	}
	stale := rev - 1
	if err := s.EditPlanning(ctx, p.ID, PlanningEdit{Action: "overview", Revision: &stale}, "", 0); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
