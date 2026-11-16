package main

import (
	"context"
	"forgeos/internal/store"
	"testing"
)

func TestStartupRejectsScriptAbsentFromPinnedCommit(t *testing.T) {
	_, job, _ := fixture(t)
	cfg := `{"version":1,"local_environments":{"local-mock":{"start_script":"start.sh","port":8080,"health_path":"/ready","timeout_seconds":10}}}`
	commit := validationCommit(t, job.RepoPath, "project.yaml", cfg)
	validationCommit(t, job.RepoPath, "start.sh", "exit 0\n")
	run := environmentRun{EnvironmentRequest: store.EnvironmentRequest{Repo: "project", Commit: commit, Environment: "local-mock"}}
	err := run.verify(context.Background(), job.RepoPath)
	if err == nil || err.Error() != "start script must be a committed regular file" || run.ConfigBlob == "" || run.Probes != 0 || run.ScriptBlob != "" {
		t.Fatal(run, err)
	}
}
