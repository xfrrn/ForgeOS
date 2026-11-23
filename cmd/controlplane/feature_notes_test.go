package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
	"github.com/google/uuid"
)

func TestFeatureNotesBeyondHistoryWindow(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	profile := uuid.New()
	if _, err = db.ExecContext(ctx, "INSERT INTO chat_models(id,model,revision) VALUES($1,'api',$2)", profile, uuid.New()); err != nil {
		t.Fatal(err)
	}
	topic, err := data.CreateFeature(ctx, project.ID, uuid.New(), profile, "预览")
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateFeature(ctx, project.ID, uuid.New(), profile, "筛选")
	if err != nil {
		t.Fatal(err)
	}
	var source int64
	err = db.QueryRowContext(ctx, `INSERT INTO project_chat_turns(project_id,feature_id,client_id,text,status,answer)
        VALUES($1,$2,gen_random_uuid(),'early-source','completed','original reply') RETURNING id`, project.ID, topic.ID).Scan(&source)
	if err != nil {
		t.Fatal(err)
	}
	agreement := store.FeatureNote{Turn: source, Kind: "agreement", Body: "owner-approved-marker"}
	key := uuid.New()
	n, err := data.AddFeatureNote(ctx, project.ID, topic.ID, key, agreement)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := data.AddFeatureNote(ctx, project.ID, topic.ID, key, agreement)
	if err != nil || replay.ID != n.ID {
		t.Fatal("replayed note", replay, err)
	}
	if _, err = data.AddFeatureNote(ctx, project.ID, other.ID, uuid.New(), agreement); !errors.Is(err, store.ErrConflict) {
		t.Fatal("cross-topic source accepted", err)
	}
	if _, err = data.FeatureNoteSource(ctx, project.ID, other.ID, n.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("cross-topic source exposed", err)
	}
	q, err := data.AddFeatureNote(ctx, project.ID, topic.ID, uuid.New(), store.FeatureNote{Turn: source, Kind: "question", Body: "open-question-marker"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO project_chat_turns(project_id,feature_id,client_id,text,status,answer)
        SELECT $1,$2,gen_random_uuid(),'later round','completed','ok' FROM generate_series(1,25)`, project.ID, topic.ID); err != nil {
		t.Fatal(err)
	}
	_, err = data.SendFeatureChat(ctx, project.ID, topic.ID, uuid.NewString(), "handoff", profile)
	if err != nil {
		t.Fatal(err)
	}
	job, err := data.ClaimFeatureAwareChat(ctx, project.ID, true, "node")
	if err != nil || job == nil {
		t.Fatal(job, err)
	}
	chat := projectChat{data: data}
	for _, native := range []*store.FeatureClient{nil, {Model: "native", Backend: "claude"}} {
		job.Native = native
		prompt, err := chat.prompt(ctx, *job)
		if err != nil || !strings.Contains(prompt, agreement.Body) || !strings.Contains(prompt, q.Body) || strings.Contains(prompt, "early-source") {
			t.Fatal("handoff missing or history unbounded", err)
		}
	}
	if err = data.CloseFeatureNote(ctx, project.ID, topic.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	prompt, err := chat.prompt(ctx, *job)
	if err != nil || strings.Contains(prompt, q.Body) || !strings.Contains(prompt, agreement.Body) {
		t.Fatal("closed note still active", err)
	}
	original, err := data.FeatureNoteSource(ctx, project.ID, topic.ID, q.ID)
	if err != nil || original.Text != "early-source" || original.Answer != "original reply" {
		t.Fatal("lost source", err)
	}
}
