package acceptance

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"forgeos/schemas"
)

// Verify checks an externally supplied plan against independently resolved inputs.
func (p Plan) Verify(b Basis, lib schemas.PlaybookLibrary) error {
	if !reflect.DeepEqual(p.Basis, b) {
		return errors.New("plan basis differs from the frozen candidate or criteria")
	}
	draft := p.Draft
	draft.Cases = slices.Clone(p.Cases)
	for i := range draft.Cases {
		c := &draft.Cases[i]
		if c.ID != fmt.Sprintf("TC-%03d", i+1) || c.Result != "pending" {
			return errors.New("unexpected case identity or execution result")
		}
		c.ID, c.Result = "", ""
	}
	_, err := b.Bind(p.Session, lib, draft)
	return err
}

type CoverageItem struct {
	Criterion string   `json:"criterion_id"`
	Cases     []string `json:"case_ids"`
	State     string   `json:"state"`
	Rationale string   `json:"rationale"`
}
type Coverage struct {
	Session     string         `json:"session_id"`
	TestSession string         `json:"test_session_id"`
	Base        string         `json:"base_commit"`
	Commit      string         `json:"commit"`
	Digest      string         `json:"plan_sha256"`
	State       string         `json:"state"`
	Items       []CoverageItem `json:"items"`
}

func (p Plan) Coverage(session string, items []CoverageItem) (Coverage, error) {
	var r Coverage
	if session == "" || session == p.Session || len(items) != len(p.Basis.Criteria) {
		return r, errors.New("coverage requires an independent session and a decision for every criterion")
	}
	state := "covered"
	remaining := map[string]bool{}
	for _, c := range p.Basis.Criteria {
		remaining[c.ID] = true
	}
	cases := map[string]string{}
	for _, c := range p.Cases {
		cases[c.ID] = c.Criterion
	}
	open := map[string]bool{}
	for _, q := range p.Questions {
		open[q.Criterion] = true
	}
	for _, x := range items {
		if !remaining[x.Criterion] || strings.TrimSpace(x.Rationale) == "" || x.Cases == nil {
			return r, errors.New("missing, duplicate or unsubstantiated criterion")
		}
		delete(remaining, x.Criterion)
		seen := map[string]bool{}
		for _, id := range x.Cases {
			if seen[id] || cases[id] != x.Criterion {
				return r, errors.New("invalid case reference in coverage review")
			}
			seen[id] = true
		}
		switch x.State {
		case "covered":
			if open[x.Criterion] || len(x.Cases) == 0 {
				return r, errors.New("unresolved or untested criterion marked covered")
			}
		case "gap":
			state = "changes_requested"
		case "needs_clarification":
			if state == "covered" {
				state = "needs_clarification"
			}
		default:
			return r, errors.New("unknown coverage state")
		}
	}
	doc, err := json.Marshal(p)
	if err != nil {
		return r, err
	}
	return Coverage{Session: session, TestSession: p.Session, Base: p.Basis.Decision.Base, Commit: p.Basis.Decision.Commit, Digest: fmt.Sprintf("%x", sha256.Sum256(doc)), State: state, Items: items}, nil
}
