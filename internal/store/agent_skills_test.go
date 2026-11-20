package store

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"forgeos/internal/agents"
	"forgeos/internal/testdb"
)

func TestSelectedSkillsReachOnlyTheirAssignedAgent(t *testing.T) {
	t.Setenv("FORGEOS_SKILLS_DIR", t.TempDir())
	ctx := context.Background()
	s := New(testdb.Open(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	firstDoc := "---\r\nname: delivery-check\r\ndescription: Check the handoff\r\n---\r\nCHECK-V1"
	first, err := s.ImportAgentSkill(ctx, firstDoc)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(first.Entry); err != nil || string(raw) != firstDoc {
		t.Fatal("SKILL.md must be stored byte for byte")
	}
	again, err := s.ImportAgentSkill(ctx, firstDoc)
	if err != nil || first.ID != again.ID {
		t.Fatalf("duplicate import: %v", err)
	}
	second, err := s.ImportAgentSkill(ctx, strings.Replace(firstDoc, "CHECK-V1", "CHECK-V2", 1))
	if err != nil || second.ID == first.ID {
		t.Fatalf("skill version not pinned: %v", err)
	}
	if _, err = s.ImportAgentSkill(ctx, "no metadata"); !errors.Is(err, ErrSkillDocument) {
		t.Fatal("accepted invalid document")
	}

	d, _ := agents.Lookup("management-chat")
	settings := agents.Settings{Prompt: d.Prompt, Harness: "claude", Skills: []int64{first.ID}}
	if err = s.SaveAgentConfig(ctx, 0, d, 0, 0, &settings); err != nil {
		t.Fatal(err)
	}
	claim := func(key string) *agents.Assignment {
		t.Helper()
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		a, err := freezeAgent(ctx, tx, project.ID, key, "skill-node", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		return a
	}
	frozen := claim(d.Key)
	prompt := frozen.Apply(d.Prompt + "\nJob context")
	if !strings.Contains(prompt, first.Entry) || strings.Contains(prompt, "CHECK-V1") || !strings.HasSuffix(prompt, "Job context") || strings.Contains(prompt, second.Entry) {
		t.Fatal("incorrect inherited skill instructions")
	}
	if strings.Contains(claim("reviewer").Prompt, first.Entry) {
		t.Fatal("skill leaked into another role")
	}
	entry, err := agentConfig(ctx, s.db, project.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]int64{{-1}, {first.ID, first.ID}, {first.ID, second.ID}} {
		settings.Skills = bad
		if err = s.SaveAgentConfig(ctx, project.ID, d, 0, entry.Effective.Revision, &settings); !errors.Is(err, ErrSkillSelection) {
			t.Fatalf("accepted %v: %v", bad, err)
		}
	}
	settings.Skills = []int64{}
	if err = s.SaveAgentConfig(ctx, project.ID, d, 0, entry.Effective.Revision, &settings); err != nil {
		t.Fatal(err)
	}
	if a := claim(d.Key); len(a.Skills) != 0 || strings.Contains(a.Prompt, first.Entry) {
		t.Fatal("explicit empty project selection inherited global skills")
	}
	if err = s.ObserveAgent(ctx, AgentObservation{Ticket: frozen.Ticket, Harness: "claude", Outcome: "completed"}); err != nil {
		t.Fatal(err)
	}
	entries, err := s.AgentEntries(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Key == d.Key {
			if e.Last == nil || !reflect.DeepEqual(e.Last.Config.Skills, []int64{first.ID}) || e.Last.Config.Prompt != frozen.Prompt {
				t.Fatal("historical skill selection changed")
			}
			if err = s.SaveAgentConfig(ctx, project.ID, d, e.LocalRevision, e.Effective.Revision, nil); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(claim(d.Key).Prompt, first.Entry) {
				t.Fatal("reset failed to restore inherited skill")
			}
			return
		}
	}
	t.Fatal("missing chat configuration")
}
