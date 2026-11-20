package runtimeinfo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsNpmShimVersion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "node & tools")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "npm.cmd")
	if err := os.WriteFile(shim, []byte("@if not \"%1\"==\"--version\" exit /b 1\r\n@echo 11.12.1\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	path := toolPath("npm")
	if path != shim {
		t.Fatalf("resolved npm to %q, want %q", path, shim)
	}
	workDir, executable, args := versionCommand(toolCommand{"npm", "--version"}, path)
	out, err := probe(context.Background(), workDir, executable, args...)
	if err != nil || readVersion(out) != "11.12.1" {
		t.Fatalf("npm version probe returned %q, %v", out, err)
	}
}
