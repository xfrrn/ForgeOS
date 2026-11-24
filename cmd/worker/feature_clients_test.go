package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscussionWorkspaceUsesProjectPath(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("LocalAppData", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	repo := t.TempDir()
	ws, err := discussionWorkspace(managementChatJob{ProjectID: 1, FeatureID: 2, ProjectPath: repo}, "prof-1")
	if err != nil || ws != filepath.Clean(repo) {
		t.Fatalf("expected repo workspace %q, got %q err=%v", repo, ws, err)
	}
	fallback, err := discussionWorkspace(managementChatJob{ProjectID: 1, FeatureID: 2}, "prof-1")
	if err != nil || !strings.Contains(fallback, "discussion-workspaces") {
		t.Fatalf("expected fallback workspace, got %q err=%v", fallback, err)
	}
}
