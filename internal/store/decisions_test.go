package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDecisionIndexRechecksProjectAndStatusAfterEncoding(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var project, decision int64
	for _, name := range []string{"this-project", "other-project"} {
		p, err := s.CreateProject(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := s.ProposeDecision(ctx, p.ID, DecisionProposal{RequestKey: "policy", Statement: name, Source: "minutes", Validity: "current", EvidenceRefs: []string{"manual"}})
		if err != nil {
			t.Fatal(err)
		}
		var d struct{ ID int64 }
		if err = json.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Decide(ctx, p.ID, d.ID, "CONFIRMED"); err != nil {
			t.Fatal(err)
		}
		if name == "this-project" {
			project, decision = p.ID, d.ID
		}
	}
	encoder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Input []string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Input) != 1 || body.Input[0] != "this-project\ncurrent" {
			t.Error("unexpected encoder input", body.Input, err)
		}
		if _, err := s.Decide(ctx, project, decision, "EXPIRED"); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0]}]}`))
	}))
	defer encoder.Close()
	s.embeddings = &materialEncoder{endpoint: encoder.URL, model: "test", client: encoder.Client()}
	if err := s.indexDecisions(ctx, project, "CONFIRMED", 2); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM decision_embeddings").Scan(&count); err != nil || count != 0 {
		t.Fatal("indexed an expired or out-of-project decision", count, err)
	}
	s.embeddings = nil
	raw, err := s.SearchDecisions(ctx, project, "EXPIRED", "minutes")
	var hits []struct{ ID int64 }
	if err != nil || json.Unmarshal(raw, &hits) != nil || len(hits) != 1 || hits[0].ID != decision {
		t.Fatal("keyword search lost the source or project scope", err, hits)
	}
}

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
