package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSingleExperienceRevisionPreservesSiblingNumbersAndScope(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	other, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	material := MaterialInput{Kind: "material", Title: "手册", Source: "Owner", Content: "Alpha Beta Gamma"}
	_, err = s.SaveMaterial(ctx, project.ID, "manual", material)
	must(err)
	review := func(facts []KnowledgeFact, verdict string) int64 {
		t.Helper()
		j, err := s.ClaimCuration(ctx, project.ID, false, false)
		must(err)
		if j == nil {
			t.Fatal("missing extraction")
		}
		must(s.RecordExtraction(ctx, project.ID, j.ID, j.Lease, Extraction{Session: "extract", Facts: facts}))
		job, err := s.ClaimCuration(ctx, project.ID, true, false)
		must(err)
		if job == nil || job.ID != j.ID {
			t.Fatal("missing review")
		}
		must(s.ReviewCuration(ctx, project.ID, job.ID, job.Lease, ManagementReview{Verdict: verdict, Level: "D1", Rationale: "Verified.", Evidence: facts[0].Quote, SessionID: "reviewer"}))
		return j.ID
	}
	original := review([]KnowledgeFact{{"one", "Alpha"}, {"two", "Beta"}, {"three", "Gamma"}}, "approve")
	var entry int64
	must(s.db.QueryRowContext(ctx, "SELECT entry_id FROM experience_revision_facts WHERE curation_id=$1 AND fact_number=2", original).Scan(&entry))
	input := ExperienceChangeInput{RequestKey: "change1", ExpectedRevision: 1, Title: "勘误", Source: "更正记录", Content: "Delta Epsilon", Validity: "批量导入", Reason: "补充验证"}
	type summary struct {
		ID        int64
		Status    string
		Current   int `json:"current_revision"`
		Revisions []struct{ Statement string }
		Changes   []struct {
			ID     int64
			Status string
		}
	}
	decode := func(raw json.RawMessage, err error) summary {
		t.Helper()
		must(err)
		var d summary
		must(json.Unmarshal(raw, &d))
		return d
	}
	first := decode(s.SubmitExperienceChange(ctx, project.ID, entry, input))
	second := decode(s.SubmitExperienceChange(ctx, project.ID, entry, input))
	if first.ID != entry || len(second.Changes) != 1 || first.Status != "current" {
		t.Fatal(first, second)
	}
	change := first.Changes[0].ID
	if _, err = s.AdoptExperienceChange(ctx, project.ID, entry, change, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("adopted before review", err)
	}
	if _, err = s.ExperienceEntry(ctx, other.ID, entry); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-project read", err)
	}
	corrected := review([]KnowledgeFact{{"first proposal", "Delta"}, {"selected correction", "Epsilon"}}, "approve")
	if _, err = s.ExperienceFact(ctx, project.ID, corrected, 1); !errors.Is(err, ErrNotFound) {
		t.Fatal("approved but unselected correction visible", err)
	}
	hits, err := s.SearchExperience(ctx, project.ID, "proposal")
	must(err)
	if string(hits) != "[]" {
		t.Fatal(string(hits))
	}
	out := decode(s.AdoptExperienceChange(ctx, project.ID, entry, change, 2))
	if out.Current != 2 || len(out.Revisions) != 2 || out.Revisions[0].Statement != "selected correction" {
		t.Fatal(out)
	}
	replayed := decode(s.AdoptExperienceChange(ctx, project.ID, entry, change, 2))
	if replayed.Current != 2 || len(replayed.Revisions) != 2 {
		t.Fatal("replay appended extra revision")
	}
	if _, err = s.AdoptExperienceChange(ctx, project.ID, entry, change, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("changed prior selection", err)
	}
	if _, err = s.ExperienceFact(ctx, project.ID, original, 2); !errors.Is(err, ErrNotFound) {
		t.Fatal("old revision visible", err)
	}
	third, err := s.ExperienceFact(ctx, project.ID, original, 3)
	must(err)
	if third.Content != "Gamma" {
		t.Fatal("original sibling lost ordinal", third)
	}
	selected, err := s.ExperienceFact(ctx, project.ID, corrected, 2)
	must(err)
	if selected.Validity != input.Validity {
		t.Fatal("missing reviewed applicability", selected)
	}
	hits, err = s.SearchExperience(ctx, project.ID, "Gamma")
	must(err)
	var facts []struct {
		Number int `json:"fact_number"`
	}
	must(json.Unmarshal(hits, &facts))
	if len(facts) != 1 || facts[0].Number != 3 {
		t.Fatal(string(hits))
	}
	page, err := s.ProjectExperience(ctx, project.ID, 0)
	must(err)
	if strings.Contains(string(page), "Beta") || strings.Contains(string(page), "first proposal") || !strings.Contains(string(page), "Epsilon") {
		t.Fatal(string(page))
	}
	if _, err = s.WithdrawExperience(ctx, project.ID, entry, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("stale withdrawal", err)
	}
	mustDiscard := input
	mustDiscard.RequestKey = "discard"
	mustDiscard.ExpectedRevision = 2
	res := decode(s.SubmitExperienceChange(ctx, project.ID, entry, mustDiscard))
	res = decode(s.DiscardExperienceChange(ctx, project.ID, entry, res.Changes[0].ID))
	if res.Changes[0].Status != "discarded" {
		t.Fatal(res)
	}
	res = decode(s.WithdrawExperience(ctx, project.ID, entry, 2))
	if res.Status != "withdrawn" || len(res.Revisions) != 2 {
		t.Fatal(res)
	}
	if _, err = s.ExperienceFact(ctx, project.ID, corrected, 2); !errors.Is(err, ErrNotFound) {
		t.Fatal("withdrawn still visible", err)
	}
	page, err = s.ExperienceEntries(ctx, project.ID, 0, "current", "")
	must(err)
	if strings.Contains(string(page), "selected correction") {
		t.Fatal(string(page))
	}
	material.PreviousRevision = 1
	material.Content = "replacement"
	_, err = s.SaveMaterial(ctx, project.ID, "manual", material)
	must(err)
	if _, err = s.ExperienceFact(ctx, project.ID, original, 3); !errors.Is(err, ErrNotFound) {
		t.Fatal("stale source still visible", err)
	}
	page, err = s.ExperienceEntries(ctx, project.ID, 0, "history", "Gamma")
	must(err)
	if !strings.Contains(string(page), `"status": "stale"`) {
		t.Fatal(string(page))
	}
}

func TestNewExperienceRequestIsPrivateAndCanBeRejected(t *testing.T) {
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
	input := ExperienceChangeInput{RequestKey: "new1", Title: "draft", Source: "field", Content: "Private draft"}
	raw, err := s.SubmitExperienceChange(ctx, p.ID, 0, input)
	must(err)
	var doc struct {
		ID      int64
		Status  string
		Changes []struct{ ID int64 }
	}
	must(json.Unmarshal(raw, &doc))
	if doc.ID <= 0 || doc.Status != "pending" {
		t.Fatal(string(raw))
	}
	retry, err := s.SubmitExperienceChange(ctx, p.ID, 0, input)
	must(err)
	if string(retry) != string(raw) {
		t.Fatal("non-idempotent new entry")
	}
	input.Content = "different"
	if _, err = s.SubmitExperienceChange(ctx, p.ID, 0, input); !errors.Is(err, ErrConflict) {
		t.Fatal("different idempotent payload accepted", err)
	}
	rows, err := s.SearchMaterials(ctx, p.ID, "", "Private", "")
	must(err)
	if string(rows) != "[]" {
		t.Fatal("pending text in CLI materials", string(rows))
	}
	j, err := s.ClaimCuration(ctx, p.ID, false, false)
	must(err)
	must(s.RecordExtraction(ctx, p.ID, j.ID, j.Lease, Extraction{Session: "extract", Facts: []KnowledgeFact{{"draft", "Private"}}}))
	j, err = s.ClaimCuration(ctx, p.ID, true, false)
	must(err)
	must(s.ReviewCuration(ctx, p.ID, j.ID, j.Lease, ManagementReview{Verdict: "revise", Level: "D1", Rationale: "Need more evidence.", Evidence: "Private", SessionID: "reviewer"}))
	if _, err = s.AdoptExperienceChange(ctx, p.ID, doc.ID, doc.Changes[0].ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("rejected revision adopted", err)
	}
	rows, err = s.ExperienceEntries(ctx, p.ID, 0, "history", "draft")
	must(err)
	if !strings.Contains(string(rows), `"status": "rejected"`) {
		t.Fatal(string(rows))
	}
}

func TestRacingExperienceChangesAdoptExactlyOneRevision(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	type response struct {
		ID      int64
		Changes []struct{ ID int64 }
	}
	input := ExperienceChangeInput{RequestKey: "first", Title: "现场", Source: "复测", Content: "首次通过"}
	type result struct {
		body json.RawMessage
		err  error
	}
	results := make(chan result, 2)
	submit := func() { body, err := s.SubmitExperienceChange(ctx, p.ID, 0, input); results <- result{body, err} }
	go submit()
	go submit()
	var original response
	for range 2 {
		x := <-results
		if x.err != nil {
			t.Fatal(x.err)
		}
		var doc response
		if err = json.Unmarshal(x.body, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Changes) != 1 || original.ID != 0 && original.ID != doc.ID {
			t.Fatal("duplicate draft", doc)
		}
		original = doc
	}
	input.RequestKey, input.Reason = "second", "独立复测"
	second, err := s.SubmitExperienceChange(ctx, p.ID, original.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	var edited response
	if err = json.Unmarshal(second, &edited); err != nil {
		t.Fatal(err)
	}
	if len(edited.Changes) != 2 {
		t.Fatal(edited)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE material_curations SET extraction='{"session":"curator","facts":[{"statement":"通过","quote":"通过"}]}',
		review='{"level":"D1","verdict":"approve"}',reviewed_at=now() WHERE project_id=$1 AND experience_change_id IS NOT NULL`, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range edited.Changes {
		go func(change int64) {
			body, err := s.AdoptExperienceChange(ctx, p.ID, original.ID, change, 1)
			results <- result{body, err}
		}(ch.ID)
	}
	won, conflict := 0, 0
	for range 2 {
		x := <-results
		switch {
		case x.err == nil:
			won++
		case errors.Is(x.err, ErrConflict):
			conflict++
		default:
			t.Fatal(x.err)
		}
	}
	if won != 1 || conflict != 1 {
		t.Fatalf("adoptions=%d conflicts=%d", won, conflict)
	}
	var revisions, obsolete int
	if err = s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM experience_revisions WHERE entry_id=$1),
		(SELECT count(*) FROM experience_change_states WHERE entry_id=$1 AND status='obsolete')`, original.ID).Scan(&revisions, &obsolete); err != nil || revisions != 1 || obsolete != 1 {
		t.Fatal(revisions, obsolete, err)
	}
}
