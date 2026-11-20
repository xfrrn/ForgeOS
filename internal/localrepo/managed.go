package localrepo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const managedPrefix = "managed://"

// ManagedRoot returns the configured base in this process. Reading the configuration
// does not create the directory, including when the default is in use.
func ManagedRoot() string {
	root := os.Getenv("FORGEOS_MANAGED_REPO_ROOT")
	if root == "" {
		var config string
		if runtime.GOOS == "windows" {
			config = os.Getenv("LOCALAPPDATA")
		} else {
			config, _ = os.UserConfigDir()
		}
		if filepath.IsAbs(config) {
			root = filepath.Join(config, "ForgeOS", "repositories")
		}
	}
	if !filepath.IsAbs(root) {
		return ""
	}
	return filepath.Clean(root)
}

func realDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("expected an absolute directory")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("expected a directory")
	}
	return real, nil
}

func managedRelativePath(relative string) (string, error) {
	if relative == "." || !fs.ValidPath(relative) || strings.ContainsAny(relative, "\\:\x00\r\n\t") {
		return "", errors.New("invalid managed repository path")
	}
	for _, part := range strings.Split(relative, "/") {
		if strings.EqualFold(part, ".git") || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", errors.New("invalid managed repository path")
		}
	}
	return filepath.Localize(relative)
}

func managedRootFor(path string) string {
	root, err := realDirectory(ManagedRoot())
	if err != nil || !within(root, path) {
		return ""
	}
	return root
}

func preferredRepoRoot(regular, path string) string {
	managed := managedRootFor(path)
	if regular != "" && within(regular, path) && (managed == "" || within(managed, regular)) {
		return regular
	}
	return managed
}

// ContainingRoot returns the most specific configured repository root for an existing
// absolute path. Callers use it to keep metadata in the same root as its working tree.
func ContainingRoot(regularRoot, path string) string {
	if !filepath.IsAbs(path) {
		return ""
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	root, _ := realDirectory(regularRoot)
	return preferredRepoRoot(root, real)
}

// ManagedRepoPath translates an existing local path to its portable managed path.
func ManagedRepoPath(repo string) string {
	if !filepath.IsAbs(repo) {
		return ""
	}
	real, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return ""
	}
	root := managedRootFor(real)
	if root == "" {
		return ""
	}
	return managedWirePath(root, real)
}

func managedWirePath(root, real string) string {
	rel, err := filepath.Rel(root, real)
	if err != nil {
		return ""
	}
	portable := filepath.ToSlash(rel)
	if _, err := managedRelativePath(portable); err != nil {
		return ""
	}
	return managedPrefix + portable
}

// ResolveManagedDirectory creates and resolves an approved import parent. A blank
// value selects the managed root. A user-supplied parent must stay below that root.
func ResolveManagedDirectory(value string) (string, error) {
	base := ManagedRoot()
	if base == "" {
		return "", errors.New("managed repository root is not configured as an absolute directory")
	}
	value = strings.TrimSpace(value)
	relative := value
	if filepath.IsAbs(value) {
		relative = ""
		actualBase, _ := realDirectory(base)
		for _, candidate := range []string{base, actualBase} {
			if candidate != "" && within(candidate, value) {
				relative, _ = filepath.Rel(candidate, value)
				break
			}
		}
		if relative == "" {
			return "", errors.New("directory is outside the managed repository root")
		}
		if relative == "." {
			relative = ""
		}
	}
	if relative != "" {
		local, err := managedRelativePath(filepath.ToSlash(relative))
		if err != nil {
			return "", err
		}
		relative = local
	}
	// Check the existing ancestry before creating even the first missing directory.
	for parent := base; ; parent = filepath.Dir(parent) {
		real, err := realDirectory(parent)
		if err == nil {
			if err := outsideGitWorktrees(real); err != nil {
				return "", err
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(parent) == parent {
			return "", errors.New("managed repository root is unavailable")
		}
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", errors.New("could not create the managed repository root")
	}
	root, err := realDirectory(base)
	if err != nil {
		return "", errors.New("managed repository root is unavailable")
	}
	if err := outsideGitWorktrees(root); err != nil {
		return "", err
	}
	if relative == "" {
		return root, nil
	}
	dir := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		next := filepath.Join(dir, part)
		if err := os.Mkdir(next, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", errors.New("could not create the managed repository directory")
		}
		actual, err := realDirectory(next)
		if err != nil {
			return "", errors.New("managed repository directory is unavailable")
		}
		rel, err := filepath.Rel(next, actual)
		if err != nil || rel != "." {
			return "", errors.New("managed repository directory cannot pass through a symlink or junction")
		}
		if err := outsideGitWorktrees(actual); err != nil {
			return "", err
		}
		dir = actual
	}
	return dir, nil
}

func outsideGitWorktrees(path string) error {
	for {
		if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
			return errors.New("managed repository directory cannot be inside an existing Git worktree")
		} else if !errors.Is(err, os.ErrNotExist) {
			return errors.New("could not inspect the managed repository directory")
		}
		// A bare repository has a HEAD file and an objects directory at its root.
		if head, headErr := os.Lstat(filepath.Join(path, "HEAD")); headErr == nil && head.Mode().IsRegular() {
			if objects, objErr := os.Stat(filepath.Join(path, "objects")); objErr == nil && objects.IsDir() {
				return errors.New("managed repository directory cannot be inside existing Git storage")
			}
		}
		parent := filepath.Dir(path)
		if path == parent {
			return nil
		}
		path = parent
	}
}
