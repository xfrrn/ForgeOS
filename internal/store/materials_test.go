package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMaterialRevisionIsolation(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.CreateProject(ctx, "second")
	if err != nil {
		t.Fatal(err)
	}
	key := "私有资料"
	m := MaterialInput{Kind: "material", Title: "Shared title", Source: "user import", Content: "旧版样本"}
	first, err := s.SaveMaterial(ctx, p.ID, key, m)
	if err != nil {
		t.Fatal(err)
	}
	m.Content = "other project"
	if _, err = s.SaveMaterial(ctx, q.ID, key, m); err != nil {
		t.Fatal(err)
	}
	m.PreviousRevision = 1
	m.Content = "updated material"
	second, err := s.SaveMaterial(ctx, p.ID, key, m)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.SaveMaterial(ctx, p.ID, key, m)
	if err != nil || string(retry) != string(second) {
		t.Fatal(err)
	}
	m.Content = "conflicting retry"
	if _, err = s.SaveMaterial(ctx, p.ID, key, m); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	m.PreviousRevision = 4
	if _, err = s.SaveMaterial(ctx, p.ID, key, m); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	previous, err := s.Material(ctx, p.ID, key, 1)
	if err != nil || string(previous) != string(first) {
		t.Fatal(err)
	}
	for _, term := range []string{"旧版", "other project"} {
		results, err := s.SearchMaterials(ctx, p.ID, "material", term, "")
		if err != nil || string(results) != "[]" {
			t.Fatal(string(results), err)
		}
	}
	results, err := s.SearchMaterials(ctx, p.ID, "material", "updated", "")
	var hits []struct {
		ProjectID int64 `json:"project_id"`
		Revision  int
	}
	if err != nil || json.Unmarshal(results, &hits) != nil || len(hits) != 1 || hits[0].Revision != 2 || hits[0].ProjectID != p.ID {
		t.Fatal(hits, err)
	}
}

func TestMaterialLiteralExcerptReferencesOriginalCharacters(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "source")
	if err != nil {
		t.Fatal(err)
	}
	longQuery := strings.Repeat("a", 512)
	prefix := strings.Repeat("文🚀", 400)
	body := prefix + "DEP[17].v2 " + longQuery
	_, err = s.SaveMaterial(ctx, p.ID, "setup", MaterialInput{Kind: "environment", Title: "Setup", Source: "inventory", Content: body})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"dep[17].V2", longQuery, "Setup", ""} {
		rows, err := s.SearchMaterials(ctx, p.ID, "environment", query, "")
		var hits []struct {
			Start   int `json:"start_char"`
			End     int `json:"end_char"`
			Excerpt string
		}
		if err != nil || json.Unmarshal(rows, &hits) != nil || len(hits) != 1 {
			t.Fatalf("query %q: %s, %v", query, rows, err)
		}
		hit := hits[0]
		if hit.Excerpt != string([]rune(body)[hit.Start:hit.End]) {
			t.Fatalf("excerpt differs from original characters [%d,%d)", hit.Start, hit.End)
		}
		if query == "" || query == "Setup" {
			if hit.Start != 0 || hit.End != 500 {
				t.Fatalf("expected opening excerpt, got [%d,%d)", hit.Start, hit.End)
			}
		} else if !strings.Contains(strings.ToLower(hit.Excerpt), strings.ToLower(query)) {
			t.Fatalf("excerpt omits %q", query)
		}
	}
}
