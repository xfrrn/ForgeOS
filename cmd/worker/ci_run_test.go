package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/validation"
)

func TestDeclaredChecksInLocalContainer(t *testing.T) {
	if os.Getenv("FORGEOS_TEST_DOCKER") != "1" {
		t.Skip("enable Docker to exercise declared checks")
	}
	_, job, _ := fixture(t)
	repo := job.RepoPath
	cfg := `version: 1
checks:
  report:
    type: static
    command: ["sh","-c","printf checked > receipt && test -f README.md"]
    timeout_seconds: 10
    reports: [receipt]
  advisory:
    type: static
    command: ["sh","-c","exit 9"]
    timeout_seconds: 10
    required: false
profiles:
  fast:
    checks: [report, advisory]
`
	base := validationCommit(t, repo, "validation.yaml", cfg)
	candidate := validationCommit(t, repo, "README.md", "candidate\n")
	out := t.TempDir()
	validate := func(base, candidate string) validation.Run {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		run, err := runLocalCI(ctx, ciRequest{Repo: repo, Base: base, Commit: candidate, Level: "V1"}, filepath.Dir(repo), out, "test-runner")
		if err != nil {
			t.Fatalf("candidate %s: %v", candidate, err)
		}
		return run
	}
	run := validate(base, candidate)
	if run.Failure != "" || len(run.Checks) != 2 || len(run.Checks[0].Outputs) != 1 || run.Checks[1].Failure != "CODE_FAILURE" || run.Checks[1].ExitCode == nil || *run.Checks[1].ExitCode != 9 {
		t.Fatalf("CI: %+v", run)
	}
	artifact := run.Checks[0].Outputs[0]
	data, err := os.ReadFile(filepath.Join(out, run.ID, artifact.File))
	hash := sha256.Sum256([]byte("checked"))
	if err != nil || string(data) != "checked" || artifact.SHA256 != hex.EncodeToString(hash[:]) {
		t.Fatalf("output %q %v", data, err)
	}
	if _, err = os.Stat(filepath.Join(repo, "receipt")); !os.IsNotExist(err) {
		t.Fatal("runner wrote to candidate checkout")
	}
	badConfig := validationCommit(t, repo, "validation.yaml", strings.Replace(cfg, "reports: [receipt]", "reports: [absent]", 1))
	next := validationCommit(t, repo, "README.md", "next candidate\n")
	run = validate(badConfig, next)
	if run.Failure != "TEST_ENV_FAILURE" || len(run.Checks) != 2 || run.Checks[0].ExitCode == nil || *run.Checks[0].ExitCode != 0 {
		t.Fatalf("missing promised report: %+v", run)
	}
}
