package validation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyUsesFrozenRulesAndDeletedPaths(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatal(string(out))
		}
		return strings.TrimSpace(string(out))
	}
	cfg := "version: 1\nminimum_validation: V2\ncomponents:\n  ledger: {paths: [private/**], playbooks: [payment], minimum_validation: V4}\n"
	write := func(file, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, file), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	if err := os.Mkdir(filepath.Join(repo, "private"), 0700); err != nil {
		t.Fatal(err)
	}
	write("acceptance.yaml", cfg)
	write("private/notes.md", "Initial\n")
	git("add", ".")
	git("commit", "-qm", "Rules")
	base := git("rev-parse", "HEAD")
	write("guide.md", "Public\n")
	git("add", "guide.md")
	git("commit", "-qm", "Docs")
	docs := git("rev-parse", "HEAD")
	for _, tc := range []struct {
		p     Policy
		level string
	}{
		{Policy{TaskType: "documentation"}, "V2"}, {Policy{Target: "staging", Management: "V1"}, "V3"}, {Policy{Management: "V4"}, "V4"},
	} {
		d, err := Assess(ctx, repo, base, docs, tc.p)
		if err != nil || d.Level != tc.level {
			t.Fatal(d, err)
		}
	}
	git("mv", "private/notes.md", "renamed.md")
	git("commit", "-qm", "Rename")
	head := git("rev-parse", "HEAD")
	write("acceptance.yaml", strings.ReplaceAll(cfg, "V4", "V1"))
	d, err := Assess(ctx, repo, base, head, Policy{Minimum: "V1"})
	if err != nil || d.Level != "V4" || len(d.Matches) != 1 || d.Matches[0].Example != "private/notes.md" || d.Changed != 3 {
		t.Fatal(d, err)
	}
	if _, err = Assess(ctx, repo, base, head, Policy{Target: "production"}); err == nil {
		t.Fatal("unsupported target accepted")
	}
}
