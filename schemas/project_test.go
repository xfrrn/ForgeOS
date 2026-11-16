package schemas

import (
	"os"
	"testing"
)

func TestProjectHarness(t *testing.T) {
	raw, err := os.ReadFile("project.yaml.example")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := ProjectHarness(raw); err != nil || value != "codex" {
		t.Fatal(value, err)
	}
	for _, raw := range []string{"version: 2", "version: 1\nexecution: {default_harness: unknown}", "version: 1\nexecution: {default_harnes: claude}", "version: 1\n---\nversion: 1"} {
		if _, err := ProjectHarness([]byte(raw)); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
