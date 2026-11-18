package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeCommandPolicyMatchesExactLocalRequest(t *testing.T) {
	raw := `{"project_id":7,"commands":["fixed command"]}`
	if _, err := configuredCommandPolicy(raw, false); err == nil {
		t.Fatal("exec transport accepted interactive approvals")
	}
	for _, bad := range []string{`{}`, `{"project_id":7,"commands":[]}`, `{"project_id":7,"commands":["hello\nworld"]}`} {
		if _, err := configuredCommandPolicy(bad, true); err == nil {
			t.Fatal(bad)
		}
	}
	p, err := configuredCommandPolicy(raw, true)
	if err != nil || p.Project != 7 {
		t.Fatal(p, err)
	}
	workspace := t.TempDir()
	req := nativeCommandParams{Command: "fixed command", Cwd: workspace, EnvironmentID: "local", Kind: "command"}
	if !req.allowed(p.Commands, workspace) {
		t.Fatal("exact local request rejected")
	}
	for _, change := range []func(*nativeCommandParams){
		func(r *nativeCommandParams) { r.Command += " " }, func(r *nativeCommandParams) { r.Cwd = t.TempDir() },
		func(r *nativeCommandParams) { r.EnvironmentID = "remote" }, func(r *nativeCommandParams) { r.Kind = "network" },
		func(r *nativeCommandParams) { r.AdditionalPermissions = json.RawMessage(`{}`) },
		func(r *nativeCommandParams) { r.NetworkApprovalContext = json.RawMessage(`{}`) },
		func(r *nativeCommandParams) { r.ApprovalID = json.RawMessage(`"additional"`) },
	} {
		other := req
		change(&other)
		if other.allowed(p.Commands, workspace) {
			t.Fatal(other)
		}
	}
}

func TestNativePatchFreezesApprovedFileContentAndPaths(t *testing.T) {
	root := t.TempDir()
	allowed := []string{"a.md", "b.md", "new/sub.md"}
	a := filepath.Join(root, "a.md")
	if err := os.WriteFile(a, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changes := []nativeFileChange{{Path: a, Kind: json.RawMessage(`{"type":"update","move_path":"b.md"}`), Diff: "@@\n-original\n+updated\n"},
		{Path: "new/sub.md", Kind: json.RawMessage(`{"type":"add"}`), Diff: "document\n"}}
	files := prepareNativePatch(root, changes, allowed, nil)
	if len(files) != 2 || files[0].MoveTo != "b.md" || files[0].Before != fmt.Sprintf("%x", sha256.Sum256([]byte("original\n"))) {
		t.Fatal(files)
	}
	if err := os.WriteFile(a, []byte("edited\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if after := prepareNativePatch(root, changes, allowed, nil); len(after) != 2 || after[0].Before == files[0].Before {
		t.Fatal(after)
	}
	if got := prepareNativePatch(root, changes, allowed, []string{"document"}); got != nil {
		t.Fatal("published hidden text")
	}
	for _, change := range []nativeFileChange{
		{Path: a, Kind: json.RawMessage(`{"type":"add"}`)},
		{Path: "a.md", Kind: json.RawMessage(`{"type":"update","move_path":"other.md"}`)},
		{Path: "a.md", Kind: json.RawMessage(`{"type":"update","new_permission":true}`)},
		{Path: "../a.md", Kind: json.RawMessage(`{"type":"delete"}`)},
	} {
		if got := prepareNativePatch(root, []nativeFileChange{change}, allowed, nil); got != nil {
			t.Fatal(got)
		}
	}
	if err := os.Link(a, filepath.Join(root, "b.md")); err != nil {
		t.Fatal(err)
	}
	if got := prepareNativePatch(root, []nativeFileChange{{Path: a, Kind: json.RawMessage(`{"type":"update"}`)}}, allowed, nil); got != nil {
		t.Fatal("writable hard link admitted")
	}
}
