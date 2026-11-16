package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestInitializationReportVersions(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	p, err := s.CreateProject(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CaptureInitializationReport(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	j, err := s.Claim(ctx, "reader", root, false)
	if err != nil || j == nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{Commit: commit}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRepositoryInventory(ctx, task.ID, j.ExecutionID, commit, "", json.RawMessage(`[{"path":"project.yaml","kind":"forgeos"}]`)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var docs [4]json.RawMessage
	for i := range docs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.CaptureInitializationReport(ctx, task.ID)
			if err != nil {
				t.Error(err)
			}
			docs[i] = r
		}()
	}
	wg.Wait()
	for _, r := range docs {
		if len(r) == 0 || string(r) != string(docs[0]) || strings.Contains(string(r), "secret_requirements") {
			t.Fatal("changed historic source", string(r))
		}
	}
	m := MaterialInput{Title: "Station profile", Kind: "profile", Source: "manual", Content: "This content is only available from the material endpoint."}
	if _, err = s.SaveMaterial(ctx, p.ID, "station", m); err != nil {
		t.Fatal(err)
	}
	r, err := s.CaptureInitializationReport(ctx, task.ID)
	if err != nil || !strings.Contains(string(r), "Station profile") || strings.Contains(string(r), m.Content) {
		t.Fatal(string(r), err)
	}
	old, err := s.InitializationReport(ctx, task.ID, 1)
	if err != nil || string(old) != string(docs[0]) {
		t.Fatal(err)
	}
	if _, err = s.InitializationReport(ctx, task.ID, 3); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	list, err := s.InitializationReports(ctx, task.ID)
	var entries []struct {
		Revision int `json:"revision"`
	}
	if err != nil || json.Unmarshal(list, &entries) != nil || len(entries) != 2 || entries[0].Revision != 2 || entries[1].Revision != 1 {
		t.Fatal(string(list), err)
	}
}
