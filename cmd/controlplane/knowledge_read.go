package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

// A material page starts at a Unicode character offset, as supplied by
// SearchMaterials. A Git page starts at the committed blob's byte offset.
const knowledgeMaterialChars = 2048
const knowledgeGitBytes = 8192

func knowledgeNumber(text string, minimum int64) (int64, bool) {
	n, err := strconv.ParseInt(text, 10, 64)
	return n, err == nil && n >= minimum && strconv.FormatInt(n, 10) == text
}

func knowledgeRead(ctx context.Context, data *store.Store, root string, project int64, commit, kind, ref string) (store.KnowledgeEvidence, error) {
	var out store.KnowledgeEvidence
	if ref == "" || len(ref) > 32<<10 || !utf8.ValidString(ref) || strings.IndexFunc(ref, unicode.IsControl) >= 0 {
		return out, store.ErrNotFound
	}
	out.Kind, out.Ref = kind, ref
	switch kind {
	case store.SharedKnowledgeKind:
		return data.ReceivedKnowledge(ctx, project, ref)
	case "decision":
		id, ok := knowledgeNumber(ref, 1)
		if !ok {
			return out, store.ErrNotFound
		}
		raw, err := data.Decision(ctx, project, id)
		if err != nil {
			return out, err
		}
		var row knowledgeDecision
		if err = json.Unmarshal(raw, &row); err != nil {
			return out, err
		}
		if row.Status != "CONFIRMED" {
			return out, store.ErrNotFound
		}
		out.KnowledgeHit = knowledgeHit(kind, ref, fmt.Sprintf("项目决策 #%d", id), row.Statement)
		out.Content, out.Validity, out.ValidUntil, out.Source, out.Version = row.Statement, row.Validity, row.Deadline, row.Source, ref
	case "experience":
		rawID, rawNumber, ok := strings.Cut(ref, ":")
		id, validID := knowledgeNumber(rawID, 1)
		number, validNumber := knowledgeNumber(rawNumber, 1)
		if !ok || !validID || !validNumber || number > 8 {
			return out, store.ErrNotFound
		}
		var err error
		out, err = data.ExperienceFact(ctx, project, id, number)
		if err != nil {
			return out, err
		}
		out.Kind, out.Ref = kind, ref
	case "material":
		parts := strings.SplitN(ref, ":", 3)
		if len(parts) != 3 || !store.ValidMaterialKey(parts[2]) {
			return out, store.ErrNotFound
		}
		rev, validRev := knowledgeNumber(parts[0], 1)
		offset, validOffset := knowledgeNumber(parts[1], 0)
		if !validRev || rev > 2147483647 || !validOffset {
			return out, store.ErrNotFound
		}
		raw, err := data.Material(ctx, project, parts[2], int(rev))
		if err != nil {
			return out, err
		}
		var row knowledgeMaterial
		if err = json.Unmarshal(raw, &row); err != nil {
			return out, err
		}
		text := []rune(row.Content)
		if offset >= int64(len(text)) {
			return out, store.ErrNotFound
		}
		end := min(offset+knowledgeMaterialChars, int64(len(text)))
		out.Content = string(text[offset:end])
		out.KnowledgeHit = knowledgeHit(kind, ref, row.Title, out.Content)
		out.Source, out.Version = row.Source, parts[0]
		out.OffsetUnit, out.Start, out.End = "char", offset, end
		if offset > 0 {
			out.FirstRef = fmt.Sprintf("%d:0:%s", rev, row.Key)
		}
		if end < int64(len(text)) {
			out.NextRef = fmt.Sprintf("%d:%d:%s", rev, end, row.Key)
		}
	case "git":
		repo, err := knowledgeRepository(ctx, data, root, project)
		if err != nil {
			return out, err
		}
		return readKnowledgeGit(ctx, repo, commit, ref)
	default:
		return out, store.ErrNotFound
	}
	return out, nil
}

// Match the literal path against the execution's committed document tree before
// reading its blob. A caller cannot substitute a revision or an arbitrary object.
func readKnowledgeGit(ctx context.Context, repo, commit, ref string) (store.KnowledgeEvidence, error) {
	var out store.KnowledgeEvidence
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) != 3 || !gitCommit.MatchString(commit) || parts[0] != commit || parts[2] == "." || !fs.ValidPath(parts[2]) || strings.IndexFunc(parts[2], unicode.IsControl) >= 0 {
		return out, store.ErrNotFound
	}
	offset, ok := knowledgeNumber(parts[1], 0)
	if !ok {
		return out, store.ErrNotFound
	}
	path := parts[2]
	files, err := localrepo.DocumentFiles(ctx, repo, commit, path)
	if err != nil {
		return out, err
	}
	if len(files) != 1 || files[0].Path != path {
		return out, store.ErrNotFound
	}
	var page strings.Builder
	src, err := localrepo.ReadDocumentParts(ctx, repo, files[0].Blob, func(_ int, base int64, part string) error {
		start := offset + int64(page.Len()) - base
		if start < 0 || start >= int64(len(part)) || page.Len() == knowledgeGitBytes {
			return nil
		}
		if !utf8.RuneStart(part[start]) {
			return store.ErrNotFound
		}
		end := min(int(start)+knowledgeGitBytes-page.Len(), len(part))
		for end > int(start) && end < len(part) && !utf8.RuneStart(part[end]) {
			end--
		}
		page.WriteString(part[start:end])
		return nil
	})
	if err != nil {
		return out, err
	}
	if src.Problem != "" || page.Len() == 0 {
		return out, store.ErrNotFound
	}
	out.Content = page.String()
	out.KnowledgeHit = knowledgeHit("git", ref, path, out.Content)
	out.Source, out.Version = path, commit
	if offset > 0 {
		out.FirstRef = fmt.Sprintf("%s:0:%s", commit, path)
	}
	end := offset + int64(page.Len())
	out.OffsetUnit, out.Start, out.End = "byte", offset, end
	if end < src.Bytes {
		out.NextRef = fmt.Sprintf("%s:%d:%s", commit, end, path)
	}
	return out, nil
}
