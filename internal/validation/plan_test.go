package validation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFixedValidationPlan(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=ForgeOS", "-c", "user.email=test@example.invalid"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git: %s", out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	raw, err := os.ReadFile("../../schemas/validation.yaml.example")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, "validation.yaml")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "validation.yaml")
	git("commit", "-qm", "Add validation configuration")
	commit, blob := git("rev-parse", "HEAD"), git("rev-parse", "HEAD:validation.yaml")
	if err = os.WriteFile(path, []byte("uncommitted: true"), 0600); err != nil {
		t.Fatal(err)
	}
	for i, n := range []int{3, 5, 6, 6} {
		level := levels[i].level
		plan, err := ReadPlan(ctx, repo, commit, level)
		if err != nil || plan.ConfigBlob != blob || len(plan.Checks) != n || plan.Ready != (i < 3) {
			t.Fatalf("%s: %+v %v", level, plan, err)
		}
		if i == 3 && !slices.Equal(plan.Missing, []string{"profile:critical", "check:security", "check:regression"}) {
			t.Fatal(plan.Missing)
		}
	}
	changed := strings.Replace(string(raw), "type: unit", "type: unit\n    required: false", 1)
	if err = os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "validation.yaml")
	git("commit", "-qm", "Mark unit optional")
	plan, err := ReadPlan(ctx, repo, git("rev-parse", "HEAD"), "V2")
	if err != nil || plan.Ready || !slices.Equal(plan.Missing, []string{"check:unit"}) {
		t.Fatal(plan, err)
	}
	if _, err = ReadPlan(ctx, repo, "HEAD", "V2"); err == nil {
		t.Fatal("mutable revision accepted")
	}
}
