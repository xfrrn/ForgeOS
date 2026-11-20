package localrepo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func initRepository(t *testing.T, path string, extra ...string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Git(context.Background(), path, append([]string{"init", "--template="}, extra...)...); err != nil {
		t.Fatal(err)
	}
}

func expectSameManagedPath(t *testing.T, got, want string) {
	t.Helper()
	if relative, err := filepath.Rel(want, got); err != nil || relative != "." {
		t.Fatalf("resolved path %q, want %q", got, want)
	}
}

func TestManagedRepoAssignmentUsesNodeRoot(t *testing.T) {
	ctx := context.Background()
	regular := t.TempDir()
	controllerManaged, workerManaged := t.TempDir(), t.TempDir()
	const alias = "managed://group/sample project"
	controllerRepo := filepath.Join(controllerManaged, "group", "sample project")
	workerRepo := filepath.Join(workerManaged, "group", "sample project")
	initRepository(t, controllerRepo)
	initRepository(t, workerRepo)
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", controllerManaged)
	if got := ManagedRepoPath(controllerRepo); got != alias {
		t.Fatalf("managed path %q, want %q", got, alias)
	}
	for _, registered := range []string{controllerRepo, alias} {
		if got := RelativePath(regular, registered); got != alias {
			t.Fatalf("relative path %q, want %q", got, alias)
		}
	}
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", workerManaged)
	for _, registered := range []string{controllerRepo, alias} {
		got, err := ResolveAssigned(ctx, regular, registered, alias)
		if err != nil {
			t.Fatal(err)
		}
		expectSameManagedPath(t, got, workerRepo)
	}
	for _, assigned := range []string{alias, workerRepo} {
		got, err := ResolveRepo(ctx, regular, assigned)
		if err != nil {
			t.Fatal(err)
		}
		expectSameManagedPath(t, got, workerRepo)
	}
	if _, err := ResolveAssigned(ctx, regular, alias, "managed://other"); err == nil {
		t.Fatal("accepted disagreeing managed paths")
	}
	for _, invalid := range []string{"managed://", "managed://.", "managed:///outside", "managed://group/..", "managed://group/.. /sample project", "managed://group\\sample project"} {
		if got := RelativePath(regular, invalid); got != "" {
			t.Errorf("serialized invalid managed path %q as %q", invalid, got)
		}
		if _, err := ResolveRepo(ctx, regular, invalid); err == nil {
			t.Errorf("accepted invalid managed path %q", invalid)
		}
	}
}

func TestManagedRepositoryMetadataStaysInManagedRoot(t *testing.T) {
	regular, managed := t.TempDir(), t.TempDir()
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", managed)
	repo := filepath.Join(managed, "worktree")
	initRepository(t, repo, "--separate-git-dir="+filepath.Join(regular, "git-store"))
	for _, path := range []string{repo, "managed://worktree"} {
		if _, err := ResolveRepo(context.Background(), regular, path); err == nil {
			t.Errorf("accepted %q with Git metadata in the other root", path)
		}
	}
}

func TestOverlappingRepoRootsUseNarrowerBoundary(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "inner")
	repo := filepath.Join(inner, "repository")
	initRepository(t, repo)
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", outer)
	if got := RelativePath(inner, repo); got != "repository" {
		t.Fatalf("outer managed root changed an existing local path: %q", got)
	}
	expectSameManagedPath(t, ContainingRoot(inner, repo), inner)
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", inner)
	if got := RelativePath(outer, repo); got != "managed://repository" {
		t.Fatalf("inner managed root was lost in broader local root: %q", got)
	}
	expectSameManagedPath(t, ContainingRoot(outer, repo), inner)
	if got := RelativePath(inner, repo); got != "repository" {
		t.Fatalf("identical configured roots changed the local path: %q", got)
	}
}

func TestResolveManagedDirectory(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "managed", "repositories")
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", base)
	expectSameManagedPath(t, ManagedRoot(), base)
	if _, err := ResolveManagedDirectory(filepath.Join(parent, "outside")); err == nil {
		t.Fatal("accepted an absolute path outside the managed root")
	}
	if _, err := os.Stat(base); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading the root and rejecting an outside path created it: %v", err)
	}
	for _, value := range []string{"", "team/demo", filepath.Join(base, "group", "another project")} {
		got, err := ResolveManagedDirectory(value)
		if err != nil {
			t.Fatal(err)
		}
		want := value
		if !filepath.IsAbs(value) {
			want = filepath.Join(base, filepath.FromSlash(value))
		}
		expectSameManagedPath(t, got, want)
		if info, err := os.Stat(got); err != nil || !info.IsDir() {
			t.Fatalf("approved directory not created: %v", err)
		}
	}
	for _, value := range []string{"../outside", "team/../../outside", "team/.. /outside", "team/.git/outside"} {
		if _, err := ResolveManagedDirectory(value); err == nil {
			t.Errorf("accepted invalid managed parent %q", value)
		}
	}
	repo := filepath.Join(base, "existing")
	initRepository(t, repo)
	if _, err := ResolveManagedDirectory(filepath.Join(repo, "nested")); err == nil {
		t.Fatal("created a managed parent in an existing worktree")
	}
	if _, err := os.Stat(filepath.Join(repo, "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created directory inside existing worktree before rejecting: %v", err)
	}
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", filepath.Join(repo, "nested-base"))
	if _, err := ResolveManagedDirectory(""); err == nil {
		t.Fatal("created a managed root in an existing worktree")
	}
	if _, err := os.Stat(filepath.Join(repo, "nested-base")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created configured root inside existing worktree before rejecting: %v", err)
	}
}

func TestManagedDirectoryAndRepoRejectSymlinkEscapes(t *testing.T) {
	regular, managed, external := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", managed)
	initRepository(t, external)
	link := filepath.Join(managed, "external")
	if runtime.GOOS == "windows" {
		// Junctions exercise Windows reparse points without requiring symlink privilege.
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `New-Item -ItemType Junction -Path $env:FORGEOS_TEST_JUNCTION -Target $env:FORGEOS_TEST_JUNCTION_TARGET -ErrorAction Stop | Out-Null`)
		cmd.Env = append(os.Environ(), "FORGEOS_TEST_JUNCTION="+link, "FORGEOS_TEST_JUNCTION_TARGET="+external)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("could not create test junction: %v\n%s", err, output)
		}
	} else if err := os.Symlink(external, link); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	if _, err := ResolveManagedDirectory("external/child"); err == nil {
		t.Fatal("accepted a managed path through an external link")
	}
	if _, err := os.Stat(filepath.Join(external, "child")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created a directory outside managed root: %v", err)
	}
	if _, err := ResolveRepo(context.Background(), regular, "managed://external"); err == nil {
		t.Fatal("accepted a working tree outside the managed root")
	}
}
