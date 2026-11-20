// Package localrepo validates the local repositories shared by the control plane and Worker.
package localrepo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git runs a fixed Git operation without inheriting repository overrides or service credentials.
func Git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return GitCommand(ctx, dir, args...).Output()
}

// GitCommand applies the same repository settings for callers that stream bounded output.
func GitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	options := []string{"--no-replace-objects", "-c", "core.hooksPath=" + os.DevNull, "-c", "core.fsmonitor=false", "-c", "protocol.allow=never", "-C", dir}
	// Administrators may share specific clones between the service accounts.
	for _, path := range filepath.SplitList(os.Getenv("FORGEOS_SHARED_REPOS")) {
		if path == dir && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.Contains(path, "*") {
			options = append(options, "-c", "safe.directory="+filepath.ToSlash(path))
			break
		}
	}
	cmd := exec.CommandContext(ctx, "git", append(options, args...)...)
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		name = strings.ToUpper(name)
		if !strings.HasPrefix(name, "GIT_") && !strings.HasPrefix(name, "FORGEOS_") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1")
	return cmd
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

// RelativePath translates the registered control-plane path for a node with another mount.
func RelativePath(root, repo string) string {
	if suffix, ok := strings.CutPrefix(repo, managedPrefix); ok {
		if _, err := managedRelativePath(suffix); err == nil {
			return repo
		}
		return ""
	}
	if !filepath.IsAbs(repo) {
		return ""
	}
	root, rootErr := realDirectory(root)
	if real, err := filepath.EvalSymlinks(repo); err == nil {
		if selected := preferredRepoRoot(root, real); selected != "" && selected != root {
			return managedWirePath(selected, real)
		}
	}
	if rootErr != nil {
		return ""
	}
	rel, err := filepath.Rel(root, repo)
	if err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func ResolveAssigned(ctx context.Context, root, registered, relative string) (string, error) {
	if strings.HasPrefix(registered, managedPrefix) {
		if relative != "" && relative != registered {
			return "", errors.New("assigned managed repository paths do not match")
		}
		return ResolveRepo(ctx, root, registered)
	}
	if strings.HasPrefix(relative, managedPrefix) {
		return ResolveRepo(ctx, root, relative)
	}
	if relative == "" {
		return ResolveRepo(ctx, root, registered)
	}
	local, err := filepath.Localize(relative)
	if err != nil {
		return "", errors.New("invalid assigned repository path")
	}
	if RelativePath(root, registered) != "" {
		// Existing deployments can give the Worker a broader root than the control plane.
		if _, err := os.Lstat(registered); !errors.Is(err, os.ErrNotExist) {
			return ResolveRepo(ctx, root, registered)
		}
	}
	return ResolveRepo(ctx, root, local)
}

func ResolveRepo(ctx context.Context, allowedRoot, path string) (string, error) {
	if !filepath.IsAbs(allowedRoot) || path == "" {
		return "", errors.New("expected an absolute repository root and a project path")
	}
	suffix, managed := strings.CutPrefix(path, managedPrefix)
	base := allowedRoot
	if managed {
		local, err := managedRelativePath(suffix)
		if err != nil {
			return "", err
		}
		base, path = ManagedRoot(), local
	}
	root, err := realDirectory(base)
	if err != nil && (managed || !filepath.IsAbs(path)) {
		return "", errors.New("repository root unavailable")
	}
	relative := !filepath.IsAbs(path)
	if relative {
		if !filepath.IsLocal(path) {
			return "", errors.New("invalid relative repository path")
		}
		path = filepath.Join(root, path)
	}
	repo, err := filepath.EvalSymlinks(path)
	if err != nil || (relative && !within(root, repo)) {
		return "", errors.New("repository path is unavailable or outside the permitted root")
	}
	if !managed {
		root = preferredRepoRoot(root, repo)
	}
	if root == "" || !within(root, repo) {
		return "", errors.New("repository path is unavailable or outside the permitted root")
	}
	out, err := Git(ctx, repo, "rev-parse", "--path-format=absolute", "--show-toplevel", "--absolute-git-dir", "--git-common-dir")
	if err != nil {
		return "", errors.New("expected a Git working tree")
	}
	paths := strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n"), "\n")
	if len(paths) != 3 {
		return "", errors.New("invalid Git repository paths")
	}
	for i, path := range paths {
		actual, err := filepath.EvalSymlinks(filepath.FromSlash(path))
		if err != nil || !within(root, actual) {
			return "", errors.New("Git metadata outside the permitted root")
		}
		if i == 0 {
			if rel, err := filepath.Rel(repo, actual); err != nil || rel != "." {
				return "", errors.New("project path must be the Git working tree root")
			}
		}
	}
	return repo, nil
}
