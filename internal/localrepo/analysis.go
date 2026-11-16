package localrepo

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

type AnalysisSource struct {
	Path  string `json:"path"`
	Blob  string `json:"blob"`
	Basis string `json:"basis"`
	Text  string `json:"text"`
}

type AnalysisInput struct {
	Sources   []AnalysisSource `json:"sources"`
	Omitted   int              `json:"omitted"`
	Truncated int              `json:"truncated"`
}

// Guides provide literal excerpts. Package summaries omit command bodies, URLs and version constraints.
func AnalysisSources(ctx context.Context, repo string, files []InventoryFile) (AnalysisInput, error) {
	ordered := append([]InventoryFile(nil), files...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if (a.Kind == "guide") != (b.Kind == "guide") {
			return a.Kind == "guide"
		}
		return strings.Count(a.Path, "/") < strings.Count(b.Path, "/")
	})
	out := AnalysisInput{Sources: []AnalysisSource{}}
	left := 64 << 10
	for _, f := range ordered {
		if f.Kind != "guide" && f.Kind != "go" && f.Kind != "node" && f.Kind != "composer" && f.Kind != "python" && f.Kind != "cargo" && f.Kind != "manifest" {
			continue
		}
		if left == 0 || len(out.Sources) == 48 {
			out.Omitted++
			continue
		}
		doc := AnalysisSource{Path: f.Path, Blob: f.Blob, Basis: "inventory"}
		limit := min(left, 16<<10)
		var truncated bool
		if f.Kind == "guide" {
			doc.Basis = "git"
			raw, large, err := LimitedGit(ctx, repo, limit, "cat-file", "blob", f.Blob)
			if err != nil {
				return out, errors.New("entry document unavailable at the checked revision")
			}
			truncated = large
			doc.Text = string(raw)
		} else {
			lines := []string{path.Base(f.Path)}
			lines = append(lines, f.Dependencies...)
			for _, name := range f.WorkspaceDependencies {
				lines = append(lines, "workspace dependency: "+name)
			}
			for _, s := range f.Scripts {
				switch f.Kind {
				case "node":
					lines = append(lines, "npm run "+s)
				case "composer":
					lines = append(lines, "composer run-script "+s)
				case "python":
					lines = append(lines, s)
				}
			}
			doc.Text = strings.Join(lines, "\n")
			if len(doc.Text) > limit {
				doc.Text = doc.Text[:limit]
				truncated = true
			}
		}
		left -= len(doc.Text)
		if truncated {
			out.Truncated++
			if i := strings.LastIndexByte(doc.Text, '\n'); i >= 0 {
				doc.Text = doc.Text[:i]
			} else {
				doc.Text = ""
			}
		}
		if !utf8.ValidString(doc.Text) || strings.ContainsRune(doc.Text, 0) || strings.TrimSpace(doc.Text) == "" {
			out.Omitted++
			continue
		}
		doc.Text = strings.ReplaceAll(doc.Text, "\r\n", "\n")
		out.Sources = append(out.Sources, doc)
	}
	return out, nil
}
