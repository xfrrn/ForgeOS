package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

const knowledgeHitLimit = 5

type knowledgeDecision struct {
	ID                                  int64
	Statement, Validity, Status, Source string
	Deadline                            string `json:"valid_until"`
}

type knowledgeExperience struct {
	ID               int64
	Number           int64 `json:"fact_number"`
	Title, Statement string
}

type knowledgeMaterial struct {
	Key, Title, Source, Content, Excerpt, Kind string
	Revision                                   int
	Start                                      int64 `json:"start_char"`
}

func knowledgeHit(kind, ref, title, excerpt string) store.KnowledgeHit {
	return store.KnowledgeHit{Kind: kind, Ref: ref, Title: chatTextLimit(title, 160), Excerpt: chatTextLimit(excerpt, 320)}
}

func knowledgePage[T any](raw json.RawMessage, err error, hit func(T) store.KnowledgeHit) ([]store.KnowledgeHit, error) {
	if err != nil {
		return nil, err
	}
	var rows []T
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	var hits []store.KnowledgeHit
	for _, row := range rows[:min(len(rows), knowledgeHitLimit)] {
		hits = append(hits, hit(row))
	}
	return hits, nil
}

func knowledgeRepository(ctx context.Context, data *store.Store, root string, project int64) (string, error) {
	p, err := data.Project(ctx, project)
	if err != nil {
		return "", err
	}
	return localrepo.ResolveRepo(ctx, root, p.Path)
}

// Each source already ranks its results; scores from different indexes aren't comparable.
// Interleave source ranks, starting with applicable decisions, up to the CLI's result budget.
func interleaveKnowledge(sources [][]store.KnowledgeHit) []store.KnowledgeHit {
	hits := make([]store.KnowledgeHit, 0, knowledgeHitLimit)
	for rank := 0; rank < knowledgeHitLimit; rank++ {
		for _, source := range sources {
			if rank < len(source) {
				hits = append(hits, source[rank])
				if len(hits) == knowledgeHitLimit {
					return hits
				}
			}
		}
	}
	return hits
}

// Without an embedding model an entire natural-language question may have no
// literal matches. Retry a bounded set of its words or Chinese phrases.
func knowledgeTerms(query string) []string {
	cleaned := chatSearchWords.Replace(strings.ToLower(query))
	cleaned = strings.NewReplacer("以前", " ", "请问", " ", "应该", " ", "我们", " ", "处理过", " ", "时，", " ", "优化", " ", "实现", " ").Replace(cleaned)
	words := strings.FieldsFunc(cleaned, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	terms := make([]string, 0, len(words))
	for _, word := range words {
		switch word {
		case "the", "and", "for", "how", "does", "what", "with", "that", "this", "are", "can", "should", "from", "project", "please":
			continue
		}
		if utf8.RuneCountInString(word) > 1 && !strings.EqualFold(word, query) && !slices.Contains(terms, word) {
			terms = append(terms, word)
		}
	}
	slices.SortStableFunc(terms, func(a, b string) int { return utf8.RuneCountInString(b) - utf8.RuneCountInString(a) })
	// Chinese task titles often have no spaces at all. The head and tail of a
	// long phrase are useful literal searches (e.g. "重复文件导入" -> "重复", "导入").
	for _, word := range words {
		runes := []rune(word)
		if len(runes) < 4 || strings.IndexFunc(word, func(r rune) bool { return !unicode.Is(unicode.Han, r) }) >= 0 {
			continue
		}
		for _, fragment := range []string{string(runes[len(runes)-2:]), string(runes[:2])} {
			if !slices.Contains(terms, fragment) {
				terms = append(terms, fragment)
			}
		}
	}
	return terms[:min(3, len(terms))]
}

func knowledgeSearch(ctx context.Context, data *store.Store, project int64, query string) ([]store.KnowledgeHit, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 512 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) {
		return nil, store.ErrConflict
	}
	sourceSearch := []func(string) ([]store.KnowledgeHit, error){
		func(term string) ([]store.KnowledgeHit, error) {
			raw, err := data.SearchDecisions(ctx, project, "CONFIRMED", term)
			return knowledgePage(raw, err, func(row knowledgeDecision) store.KnowledgeHit {
				return knowledgeHit("decision", strconv.FormatInt(row.ID, 10), fmt.Sprintf("项目决策 #%d", row.ID), row.Statement)
			})
		},
		func(term string) ([]store.KnowledgeHit, error) {
			raw, err := data.SearchExperience(ctx, project, term)
			return knowledgePage(raw, err, func(row knowledgeExperience) store.KnowledgeHit {
				return knowledgeHit("experience", fmt.Sprintf("%d:%d", row.ID, row.Number), row.Title, row.Statement)
			})
		},
		func(term string) ([]store.KnowledgeHit, error) {
			raw, err := data.SearchMaterials(ctx, project, "", term, "")
			return knowledgePage(raw, err, func(row knowledgeMaterial) store.KnowledgeHit {
				kind := "material"
				if row.Kind == "instruction" {
					kind = "instruction"
				}
				return knowledgeHit(kind, fmt.Sprintf("%d:%d:%s", row.Revision, row.Start, row.Key), row.Title, row.Excerpt)
			})
		},
		func(term string) ([]store.KnowledgeHit, error) {
			raw, err := data.SearchSharedExperience(ctx, project, term)
			return knowledgePage(raw, err, func(row struct {
				ID                     int64
				Title, Content, Digest string
			}) store.KnowledgeHit {
				return knowledgeHit(store.SharedKnowledgeKind, fmt.Sprintf("%d:%s", row.ID, row.Digest), row.Title, row.Content)
			})
		},
	}
	failures := make([]error, len(sourceSearch))
	var hits []store.KnowledgeHit
	for _, term := range append([]string{query}, knowledgeTerms(query)...) {
		lists := make([][]store.KnowledgeHit, len(sourceSearch))
		var group sync.WaitGroup
		for i, search := range sourceSearch {
			if failures[i] != nil {
				continue
			}
			group.Add(1)
			go func() {
				defer group.Done()
				lists[i], failures[i] = search(term)
			}()
		}
		group.Wait()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hits = interleaveKnowledge(lists)
		if len(hits) > 0 {
			break
		}
	}
	if !slices.Contains(failures, nil) {
		return nil, errors.Join(failures...)
	}
	return hits, errors.Join(failures...)
}
