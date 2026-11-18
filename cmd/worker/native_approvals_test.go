package main

import (
	"encoding/json"
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
