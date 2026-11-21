package localrepo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

func DocumentTree(ctx context.Context, repo, commit string) (map[string]string, error) {
	blobs, err := DocumentFiles(ctx, repo, commit)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(blobs))
	for _, b := range blobs {
		files[b.Path] = b.Blob
	}
	return files, nil
}

// DocumentFiles lists eligible documentation blobs, optionally under literal repository paths.
func DocumentFiles(ctx context.Context, repo, commit string, paths ...string) ([]TrackedBlob, error) {
	return trackedBlobs(ctx, repo, commit, func(file string) bool {
		if inventoryKind(file) == "guide" {
			return true
		}
		switch strings.ToLower(path.Ext(file)) {
		case ".md", ".mdx", ".markdown", ".rst", ".adoc", ".asciidoc":
			return true
		case ".txt":
			return strings.Contains("/"+strings.ToLower(file), "/docs/")
		}
		return false
	}, paths...)
}

func DocumentContent(ctx context.Context, repo, blob string) (content, problem string, err error) {
	content, problem, err = ReadDocumentText(ctx, repo, blob, 256<<10)
	if err == nil && problem == "" && strings.TrimSpace(content) == "" {
		return "", "empty", nil
	}
	return content, problem, err
}

// ReadDocumentText preserves the exact contents of a committed UTF-8 blob, including whitespace.
func ReadDocumentText(ctx context.Context, repo, blob string, limit int) (content, problem string, err error) {
	if !objectID.MatchString(blob) {
		return "", "", errors.New("invalid document blob")
	}
	raw, large, err := LimitedGit(ctx, repo, limit, "cat-file", "blob", blob)
	switch {
	case err != nil:
		return "", "", err
	case large:
		return "", "size_limit", nil
	case !utf8.Valid(raw) || strings.ContainsRune(string(raw), 0):
		return "", "non_text", nil
	default:
		return string(raw), "", nil
	}
}

// DocumentCommits follows the branch's first-parent history. Divergent or pruned
// history can only establish a transition between the two observed trees.
func DocumentCommits(ctx context.Context, repo, previous, head string) ([]string, error) {
	if !objectID.MatchString(head) || (previous != "" && !objectID.MatchString(previous)) {
		return nil, errors.New("invalid document commit")
	}
	if previous == "" {
		return []string{head}, nil
	}
	resolve := func(revision string) (string, error) {
		raw, err := Git(ctx, repo, "rev-parse", "--verify", "--quiet", revision+"^{commit}")
		var exit *exec.ExitError
		if ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", nil
		}
		return strings.TrimSpace(string(raw)), err
	}
	old, err := resolve(previous)
	if err != nil {
		return nil, err
	}
	if old == "" {
		return []string{head}, nil
	}
	raw, err := Git(ctx, repo, "rev-list", "--first-parent", "--count", previous+".."+head)
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, err
	}
	if n <= 1 {
		return []string{head}, nil
	}
	ancestor, err := resolve(fmt.Sprintf("%s~%d", head, n))
	if err != nil {
		return nil, err
	}
	if ancestor != previous {
		return []string{head}, nil
	}

	// Keep each durable batch small enough to resume across control-plane restarts.
	const batch = 32
	tip := head
	if n > batch {
		tip = fmt.Sprintf("%s~%d", head, n-batch)
	}
	raw, err = Git(ctx, repo, "rev-list", "--first-parent", "--reverse", fmt.Sprintf("--max-count=%d", min(n, batch)), tip)
	if err != nil {
		return nil, err
	}
	commits := strings.Fields(string(raw))
	if len(commits) != min(n, batch) {
		return nil, errors.New("incomplete document commit history")
	}
	return commits, nil
}
