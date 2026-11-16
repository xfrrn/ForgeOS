package acceptance

import (
	"encoding/json"
	"testing"

	"forgeos/internal/validation"
	"forgeos/schemas"
)

func TestPlanBindingsAndClarifications(t *testing.T) {
	lib, err := schemas.GlobalPlaybooks()
	if err != nil {
		t.Fatal(err)
	}
	m := validation.Match{Component: "ledger", Playbooks: []string{"payment", "private-ledger"}, Manual: &schemas.ManualAcceptance{Required: json.RawMessage("true"), Environment: "staging"}}
	b, err := NewBasis(validation.Decision{Matches: []validation.Match{m}}, lib, []string{"Reject duplicate payments."})
	if err != nil {
		t.Fatal(err)
	}
	makeDraft := func() Draft {
		c := Case{Criterion: "component/ledger/scope", Playbook: "payment", Objective: "Check retries", Preconditions: "Test order available", Environment: "staging", Steps: []string{"Submit twice"}, Expected: "Reject duplicate payments.", Evidence: []string{"Record order count"}, Tester: "human"}
		return Draft{Cases: []Case{c}, Questions: []Question{{"requirement/1", "Which sandbox account should be used?"}, {"component/ledger/playbook/private-ledger", "Where is the private-ledger method defined?"}}}
	}
	draft := makeDraft()
	p, err := b.Bind("agent", lib, draft)
	if err != nil || p.Cases[0].Result != "pending" || p.Cases[0].ID != "TC-001" || draft.Cases[0].ID != "" {
		t.Fatalf("binding: %+v, %v", p, err)
	}
	for _, change := range []func(*Draft){
		func(d *Draft) { d.Cases[0].Tester = "runner" }, func(d *Draft) { d.Cases[0].Environment = "local" },
		func(d *Draft) { d.Cases[0].Playbook = "api-backend" }, func(d *Draft) { d.Cases[0].Expected = "Charge two dollars" },
		func(d *Draft) { d.Cases[0].Result = "passed" }, func(d *Draft) { d.Questions = d.Questions[:1] },
		func(d *Draft) { d.Cases[0].Criterion = "missing" }, func(d *Draft) { d.Cases[0].Steps = nil },
	} {
		bad := makeDraft()
		change(&bad)
		if _, err := b.Bind("agent", lib, bad); err == nil {
			t.Fatal("accepted invalid draft", bad)
		}
	}
	empty, err := NewBasis(validation.Decision{}, lib, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = empty.Bind("agent", lib, Draft{Cases: []Case{}, Questions: []Question{{"requirement/1", "What outcome must be accepted?"}}}); err != nil {
		t.Fatal(err)
	}
}
