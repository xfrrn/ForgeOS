package localrepo

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var ErrDocumentRevision = errors.New("revision unavailable in registered history")

type DocumentMatch struct {
	Path         string `json:"path"`
	Blob         string `json:"blob"`
	Start        int64  `json:"start_byte"`
	End          int64  `json:"end_byte"`
	ExcerptStart int64  `json:"excerpt_start_byte"`
	Excerpt      string `json:"excerpt"`
}

type DocumentSearch struct {
	Commit      string          `json:"commit"`
	CommittedAt string          `json:"committed_at"`
	Items       []DocumentMatch `json:"items"`
	More        bool            `json:"more"`
}

// DocumentRevision pins HEAD once and admits historical commits reachable from it.
func DocumentRevision(ctx context.Context, repo, revision string) (string, error) {
	if revision != "" && !objectID.MatchString(revision) {
		return "", ErrDocumentRevision
	}
	raw, err := Git(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(string(raw))
	if revision == "" || revision == head {
		return head, nil
	}
	if _, err := Git(ctx, repo, "merge-base", "--is-ancestor", revision, head); err != nil {
		return "", ErrDocumentRevision
	}
	kind, err := Git(ctx, repo, "cat-file", "-t", revision)
	if err != nil || strings.TrimSpace(string(kind)) != "commit" {
		return "", ErrDocumentRevision
	}
	return revision, nil
}

// SearchDocuments pins the revision and validates the entire blob before reporting a text file.
// With a visitor, all eligible files are visited for ranking, including those without a literal match.
func SearchDocuments(ctx context.Context, repo, revision, query string, visit func(DocumentMatch) error) (DocumentSearch, error) {
	var result DocumentSearch
	if revision != "" && !objectID.MatchString(revision) {
		return result, errors.New("invalid document revision")
	}
	if len(query) == 0 || len(query) > 512 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) {
		return result, errors.New("invalid document query")
	}
	commit, err := DocumentRevision(ctx, repo, revision)
	if err != nil {
		return result, err
	}
	result.Commit = commit
	date, err := Git(ctx, repo, "show", "-s", "--no-patch", "--format=%cI", "--no-show-signature", result.Commit, "--")
	if err != nil {
		return result, err
	}
	result.CommittedAt = strings.TrimSpace(string(date))
	files, err := DocumentTree(ctx, repo, result.Commit)
	if err != nil {
		return result, err
	}
	match := regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
	result.Items = []DocumentMatch{}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		hit := DocumentMatch{Path: path, Blob: files[path]}
		var tail string
		source, err := ReadDocumentParts(ctx, repo, hit.Blob, func(_ int, offset int64, part string) error {
			if hit.Excerpt != "" {
				return nil
			}
			window := tail + part
			base := offset - int64(len(tail))
			if span := match.FindStringIndex(window); span != nil {
				start := max(0, span[0]-120)
				for !utf8.RuneStart(window[start]) {
					start++
				}
				end := min(len(window), span[1]+120)
				for end < len(window) && !utf8.RuneStart(window[end]) {
					end--
				}
				hit.Excerpt = window[start:end]
				hit.ExcerptStart = base + int64(start)
				hit.Start, hit.End = base+int64(span[0]), base+int64(span[1])
			} else {
				// A case-folded rune can take four bytes even when the query uses one.
				start := max(0, len(window)-4*utf8.RuneCountInString(query)-120)
				for !utf8.RuneStart(window[start]) {
					start++
				}
				tail = strings.Clone(window[start:])
			}
			return nil
		})
		if err != nil {
			return result, fmt.Errorf("read document blob %s: %w", hit.Blob, err)
		}
		if source.Problem != "" {
			continue
		}
		if visit != nil {
			if err := visit(hit); err != nil {
				return result, err
			}
			continue
		}
		if hit.Excerpt == "" {
			continue
		}
		if len(result.Items) == 30 {
			result.More = true
			break
		}
		result.Items = append(result.Items, hit)
	}
	return result, nil
}
