package schemas

import (
	"os"
	"strings"
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

func TestSecretRequirementsBelongToOneEnvironment(t *testing.T) {
	base := "version: 1\nsecret_requirements:\n  - {environment: ci-test, name: api-key, risk: S2}\n"
	config, err := ParseProject([]byte(base + "  - {environment: staging, name: api-key, risk: S3}\n"))
	if err != nil || len(config.SecretRequirements) != 2 || config.SecretRequirements[0] != (SecretRequirement{Environment: "ci-test", Name: "api-key", Risk: "S2"}) {
		t.Fatal(config, err)
	}
	legacy, err := ParseProject([]byte("version: 1"))
	if err != nil || legacy.SecretRequirements == nil || len(legacy.SecretRequirements) != 0 {
		t.Fatal("missing empty declaration inventory", err)
	}
	for _, raw := range []string{
		base + "  - {environment: ci-test, name: api-key, risk: S1}\n",
		strings.Replace(base, "S2}", "S2, value: fixture}", 1),
		strings.Replace(base, "S2", "S9", 1), strings.Replace(base, "ci-test", "shared", 1), strings.Replace(base, "api-key", "../api-key", 1),
		"version: 1\nsecret_requirements: null",
	} {
		if _, err := ParseProject([]byte(raw)); err == nil {
			t.Fatal("accepted invalid secret requirements")
		}
	}
}
