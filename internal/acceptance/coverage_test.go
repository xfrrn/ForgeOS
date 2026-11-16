package acceptance

import (
	"testing"

	"forgeos/internal/validation"
	"forgeos/schemas"
)

func TestIndependentCriterionCoverage(t *testing.T) {
	lib, err := schemas.GlobalPlaybooks()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBasis(validation.Decision{}, lib, []string{"Document the limit.", "Clarify the cutoff."})
	if err != nil {
		t.Fatal(err)
	}
	draft := Draft{Cases: []Case{{Criterion: "requirement/1", Playbook: "documentation", Objective: "Check limit", Preconditions: "Fixed candidate", Environment: "local", Steps: []string{"Read limit"}, Expected: "Document the limit.", Evidence: []string{"Source lines"}, Tester: "human"}}, Questions: []Question{{"requirement/2", "Which cutoff?"}}}
	p, err := b.Bind("test", lib, draft)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Verify(b, lib); err != nil {
		t.Fatal(err)
	}
	items := []CoverageItem{{"requirement/1", []string{"TC-001"}, "covered", "Steps compare the source."}, {"requirement/2", []string{}, "needs_clarification", "Cutoff missing."}}
	r, err := p.Coverage("management", items)
	if err != nil || r.State != "needs_clarification" || len(r.Digest) != 64 {
		t.Fatal(r, err)
	}
	if _, err = p.Coverage("test", items); err == nil {
		t.Fatal("self-review allowed")
	}
	items[1].State = "covered"
	if _, err = p.Coverage("manager", items); err == nil {
		t.Fatal("open question approved")
	}
	items[1].State = "gap"
	items[1].Cases = []string{"TC-001"}
	if _, err = p.Coverage("manager", items); err == nil {
		t.Fatal("cross-criterion evidence allowed")
	}
	items[1].Cases = []string{}
	gap, err := p.Coverage("management", items)
	if err != nil || gap.State != "changes_requested" || gap.Digest != r.Digest {
		t.Fatal(gap, err)
	}
	p.Cases[0].Steps = []string{"Check the file name only"}
	changed, err := p.Coverage("another-session", items)
	if err != nil || changed.Digest == r.Digest {
		t.Fatal("different plan reused digest")
	}
	p.Cases[0].Result = "passed"
	if err = p.Verify(b, lib); err == nil {
		t.Fatal("executed result passed as draft")
	}
}
