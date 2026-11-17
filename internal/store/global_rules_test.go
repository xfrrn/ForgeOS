package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGlobalRulesPublishOneImmutableRevision(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if doc, err := s.GlobalRules(ctx, 0); err != nil || string(doc) != "null" {
		t.Fatal(string(doc), err)
	}
	type result struct {
		content string
		receipt string
		err     error
	}
	results := make(chan result, 2)
	for _, content := range []string{"Default A", "Default B"} {
		go func() {
			doc, err := s.PublishGlobalRules(ctx, 0, content)
			results <- result{content, string(doc), err}
		}()
	}
	first, other := <-results, <-results
	if first.err != nil {
		first, other = other, first
	}
	if first.err != nil || !errors.Is(other.err, ErrConflict) {
		t.Fatal(first.err, other.err)
	}
	if doc, err := s.PublishGlobalRules(ctx, 0, first.content); err != nil || string(doc) != first.receipt {
		t.Fatal(string(doc), err)
	}
	if _, err := s.PublishGlobalRules(ctx, 1, ""); err != nil {
		t.Fatal(err)
	}
	if doc, err := s.GlobalRules(ctx, 1); err != nil || string(doc) != first.receipt {
		t.Fatal(string(doc), err)
	}
	if doc, err := s.GlobalRules(ctx, 0); err != nil || strings.Contains(string(doc), first.content) {
		t.Fatal("withdrawn default still current", err)
	}
	if _, err := s.PublishGlobalRules(ctx, 4, "future"); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted missing previous revision", err)
	}
}
