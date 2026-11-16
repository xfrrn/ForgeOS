package store

import (
	"context"
	"encoding/json"
	"errors"
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
