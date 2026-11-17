// Package acceptance binds proposed test cases to the requirements of a fixed candidate.
package acceptance

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"forgeos/internal/validation"
	"forgeos/schemas"
)

type Criterion struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Component string `json:"component,omitempty"`
}

type Basis struct {
	CandidateContextSHA string              `json:"candidate_context_sha256,omitempty"`
	Decision            validation.Decision `json:"decision"`
	LibrarySHA          string              `json:"library_sha256"`
	Criteria            []Criterion         `json:"criteria"`
}

func NewBasis(d validation.Decision, lib schemas.PlaybookLibrary, criteria []string) (Basis, error) {
	b := Basis{Decision: d, LibrarySHA: lib.SHA256}
	if len(criteria) > 128 {
		return b, errors.New("at most 128 requirement criteria are supported")
	}
	for i, s := range criteria {
		if strings.TrimSpace(s) == "" || len(s) > 260<<10 {
			return b, errors.New("invalid requirement criterion")
		}
		b.Criteria = append(b.Criteria, Criterion{fmt.Sprintf("requirement/%d", i+1), s, ""})
	}
	for _, m := range d.Matches {
		if len(m.Expected) == 0 {
			b.Criteria = append(b.Criteria, Criterion{"component/" + m.Component + "/scope", strings.Join(criteria, "\n"), m.Component})
		}
		for i, s := range m.Expected {
			b.Criteria = append(b.Criteria, Criterion{fmt.Sprintf("component/%s/%d", m.Component, i+1), s, m.Component})
		}
		for _, id := range m.Playbooks {
			if _, ok := lib.Playbooks[id]; !ok {
				b.Criteria = append(b.Criteria, Criterion{ID: "component/" + m.Component + "/playbook/" + id, Component: m.Component})
			}
		}
	}
	if len(b.Criteria) == 0 {
		b.Criteria = append(b.Criteria, Criterion{ID: "requirement/1"})
	}
	if len(b.Criteria) > 256 {
		return b, errors.New("split the plan into at most 256 criteria")
	}
	return b, nil
}

type Case struct {
	ID            string   `json:"id"`
	Criterion     string   `json:"criterion_id"`
	Playbook      string   `json:"playbook_id"`
	Objective     string   `json:"objective"`
	Preconditions string   `json:"preconditions"`
	Environment   string   `json:"environment"`
	Steps         []string `json:"steps"`
	Expected      string   `json:"expected_result"`
	Evidence      []string `json:"evidence"`
	Tester        string   `json:"tester"`
	Result        string   `json:"result"`
}
type Question struct {
	Criterion string `json:"criterion_id"`
	Text      string `json:"question"`
}
type Draft struct {
	Cases     []Case     `json:"cases"`
	Questions []Question `json:"clarifications"`
}
type Plan struct {
	Basis   Basis  `json:"basis"`
	Session string `json:"session_id"`
	Draft
}

var sessionID = regexp.MustCompile("^[A-Za-z0-9_-]{1,128}$")

func (b Basis) Bind(session string, lib schemas.PlaybookLibrary, draft Draft) (Plan, error) {
	p := Plan{b, session, draft}
	p.Cases = slices.Clone(draft.Cases)
	if !sessionID.MatchString(session) || b.LibrarySHA != lib.SHA256 || draft.Cases == nil || draft.Questions == nil || len(draft.Cases) > 512 || len(draft.Questions) > 256 {
		return p, errors.New("incomplete or oversized test plan")
	}
	criteria := make(map[string]Criterion, len(b.Criteria))
	for _, c := range b.Criteria {
		criteria[c.ID] = c
	}
	components := map[string]validation.Match{}
	for _, m := range b.Decision.Matches {
		components[m.Component] = m
	}
	covered := map[string]bool{}
	for i := range p.Cases {
		c := &p.Cases[i]
		source, ok := criteria[c.Criterion]
		if !ok || strings.TrimSpace(c.Expected) == "" || !strings.Contains(source.Text, c.Expected) {
			return p, fmt.Errorf("case %d requires a verbatim expected result from its criterion", i+1)
		}
		if _, ok = lib.Playbooks[c.Playbook]; !ok {
			return p, fmt.Errorf("unknown playbook %q", c.Playbook)
		}
		if m, scoped := components[source.Component]; scoped {
			if !slices.Contains(m.Playbooks, c.Playbook) {
				return p, fmt.Errorf("playbook outside component %s", m.Component)
			}
			if rule := m.Manual; rule != nil && ((string(rule.Required) == "true" && c.Tester != "human") || (rule.Environment != "" && c.Environment != rule.Environment)) {
				return p, fmt.Errorf("case contradicts manual acceptance rules for %s", m.Component)
			}
		}
		if strings.TrimSpace(c.Objective) == "" || strings.TrimSpace(c.Preconditions) == "" || strings.TrimSpace(c.Environment) == "" || !validItems(c.Steps) || !validItems(c.Evidence) || (c.Tester != "human" && c.Tester != "runner") || c.Result != "" || c.ID != "" {
			return p, fmt.Errorf("incomplete or pre-executed case %d", i+1)
		}
		c.ID = fmt.Sprintf("TC-%03d", i+1)
		c.Result = "pending"
		covered[c.Criterion] = true
	}
	for _, q := range p.Questions {
		if _, ok := criteria[q.Criterion]; !ok || strings.TrimSpace(q.Text) == "" {
			return p, errors.New("clarification requires a source criterion and question")
		}
		covered[q.Criterion] = true
	}
	for _, c := range b.Criteria {
		if !covered[c.ID] {
			return p, fmt.Errorf("criterion %s lacks a case or clarification", c.ID)
		}
	}
	return p, nil
}

func validItems(items []string) bool {
	if len(items) == 0 || len(items) > 64 {
		return false
	}
	for _, s := range items {
		if strings.TrimSpace(s) == "" {
			return false
		}
	}
	return true
}
