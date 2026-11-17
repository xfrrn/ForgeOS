package localrepo

import (
	"context"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

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

// SearchDocuments reads the registered checkout's committed history and never follows working-tree links.
func SearchDocuments(ctx context.Context, repo, revision, query string) (DocumentSearch, error) {
	var result DocumentSearch
	if revision != "" && !objectID.MatchString(revision) {
		return result, errors.New("invalid document revision")
	}
	if len(query) == 0 || len(query) > 512 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) {
		return result, errors.New("invalid document query")
	}
	head, err := Git(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return result, err
	}
	result.Commit = strings.TrimSpace(string(head))
	if revision != "" && revision != result.Commit {
		if _, err := Git(ctx, repo, "merge-base", "--is-ancestor", revision, result.Commit); err != nil {
			return result, errors.New("revision unavailable in registered history")
		}
		result.Commit = revision
	}
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
			return result, err
		}
		if source.Problem != "" || hit.Excerpt == "" {
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
