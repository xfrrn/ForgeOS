package localrepo

import (
	"context"
	"errors"
	"path"
	"strings"
	"unicode/utf8"
)

func DocumentTree(ctx context.Context, repo, commit string) (map[string]string, error) {
	blobs, err := trackedBlobs(ctx, repo, commit, func(file string) bool {
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
	})
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(blobs))
	for _, b := range blobs {
		files[b.Path] = b.Blob
	}
	return files, nil
}

func DocumentContent(ctx context.Context, repo, blob string) (content, problem string, err error) {
	if !objectID.MatchString(blob) {
		return "", "", errors.New("invalid document blob")
	}
	raw, large, err := LimitedGit(ctx, repo, 256<<10, "cat-file", "blob", blob)
	switch {
	case err != nil:
		return "", "", err
	case large:
		return "", "size_limit", nil
	case !utf8.Valid(raw) || strings.ContainsRune(string(raw), 0):
		return "", "non_text", nil
	case strings.TrimSpace(string(raw)) == "":
		return "", "empty", nil
	default:
		return string(raw), "", nil
	}
}
