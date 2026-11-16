package localrepo

import (
	"reflect"
	"testing"
)

func TestCargoInventoryFollowsDeclaredWorkspace(t *testing.T) {
	const root = "workspace/Cargo.toml"
	const member = "crates/app/Cargo.toml"
	manifests := map[string]*cargoManifest{
		root: readCargo([]byte(`[workspace.dependencies]
shared={package="actual-lib",git="https://example.invalid/private"}
unused="2"
`)),
		member: readCargo([]byte(`[package]
name="app"
workspace="../../workspace"
[dependencies]
shared.workspace=true
missing.workspace=true
[dev-dependencies]
tests="1"
[target.'cfg(windows)'.build-dependencies]
win={package="actual-win",path="../private"}
`)),
		"Cargo.toml": readCargo([]byte(`[workspace.dependencies]
shared={package="wrong-lib",version="1"}
`)),
	}
	files := []InventoryFile{{Kind: "cargo", Path: member}, {Kind: "cargo", Path: root, Blob: "workspace-blob"}}
	resolveCargo(files, manifests)
	f := files[0]
	if !reflect.DeepEqual(f.Dependencies, []string{"actual-lib", "actual-win", "tests"}) || f.UnresolvedInputs != 1 || f.WorkspaceSource == nil || *f.WorkspaceSource != (ManifestSource{Path: root, Blob: "workspace-blob"}) {
		t.Fatal(f)
	}
	if !reflect.DeepEqual(files[1].WorkspaceDependencies, []string{"actual-lib", "unused"}) || len(files[1].Dependencies) != 0 {
		t.Fatal(files[1])
	}
	manifests[member].Package.Workspace = "../../../outside"
	if cargoRoot(member, manifests[member], manifests) != "" {
		t.Fatal("escaped explicit workspace")
	}
	manifests[member].Package.Workspace = ""
	if cargoRoot(member, manifests[member], manifests) != "Cargo.toml" {
		t.Fatal("missing ancestor workspace")
	}
	manifests["Cargo.toml"].Workspace.Exclude = []string{"crates"}
	if cargoRoot(member, manifests[member], manifests) != "" {
		t.Fatal("inherited from excluded workspace")
	}
	manifests["Cargo.toml"].Workspace.Members = []string{"crates/app"}
	if cargoRoot(member, manifests[member], manifests) != "Cargo.toml" {
		t.Fatal("explicit member excluded")
	}
	manifests["crates/Cargo.toml"] = nil
	if cargoRoot(member, manifests[member], manifests) != "" {
		t.Fatal("guessed past unreadable manifest")
	}
}
