package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestConcurrentDecisionReplacement(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "decisions")
	if err != nil {
		t.Fatal(err)
	}
	add := func(key string, previous *int64, until *time.Time) int64 {
		t.Helper()
		doc, err := s.ProposeDecision(ctx, p.ID, DecisionProposal{RequestKey: key, Statement: "Use the documented procedure", Source: "meeting", Validity: "demo project", EvidenceRefs: []string{"minutes/1"}, Supersedes: previous, ValidUntil: until})
		if err != nil {
			t.Fatal(err)
		}
		var row struct{ ID int64 }
		if err = json.Unmarshal(doc, &row); err != nil || row.ID <= 0 {
			t.Fatal(err, row)
		}
		return row.ID
	}
	base := add("initial", nil, nil)
	if _, err = s.Decide(ctx, p.ID, base, "CONFIRMED"); err != nil {
		t.Fatal(err)
	}
	a, b := add("first", &base, nil), add("second", &base, nil)
	results := make(chan error, 2)
	for _, id := range []int64{a, b} {
		go func(id int64) { _, err := s.Decide(ctx, p.ID, id, "CONFIRMED"); results <- err }(id)
	}
	winners, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatal(winners, conflicts)
	}
	again, err := s.Decide(ctx, p.ID, base, "CONFIRMED")
	if err != nil {
		t.Fatal(err)
	}
	var original struct {
		Status  string
		History []struct{ Status string }
	}
	if err = json.Unmarshal(again, &original); err != nil || original.Status != "SUPERSEDED" || len(original.History) != 3 {
		t.Fatal(original, err)
	}
	ended := time.Now().Add(-time.Minute)
	past := add("time-limited", nil, &ended)
	if _, err = s.Decide(ctx, p.ID, past, "CONFIRMED"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	list, err := s.Decisions(ctx, p.ID, 0, "EXPIRED")
	var expired []struct{ ID int64 }
	if err != nil || json.Unmarshal(list, &expired) != nil || len(expired) != 1 || expired[0].ID != past {
		t.Fatal(expired, err)
	}
}
