package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestKnowledgeModelBindingGuardsClaimAndCommit(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	other, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	m := ChatModel{ID: uuid.New(), Revision: uuid.New(), Model: "test", Name: "test"}
	must(s.SaveChatModel(ctx, m, uuid.Nil, func() error { return nil }))
	before, err := s.KnowledgeModelSettings(ctx, p.ID)
	must(err)
	if len(before) != 2 || before["curator"].ModelID.Valid || before["management"].Revision != 0 {
		t.Fatal(before)
	}
	unknown := uuid.New()
	if _, err = s.SetKnowledgeModel(ctx, p.ID, "curator", 0, &unknown); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	_, err = s.SetKnowledgeModel(ctx, p.ID, "curator", 0, &m.ID)
	must(err)
	if _, err = s.SetKnowledgeModel(ctx, p.ID, "curator", 0, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	bindings, err := s.ConfiguredKnowledgeRoleModels(ctx)
	must(err)
	if len(bindings) != 1 || bindings[0].ProjectID != p.ID || bindings[0].Role != "curator" || bindings[0].ModelRevision != m.Revision {
		t.Fatal(bindings)
	}
	b := bindings[0]
	guarded := b.WithContext(ctx)
	_, err = s.SaveMaterial(ctx, p.ID, "note", MaterialInput{Title: "note", Kind: "material", Source: "owner", Content: "before rotation"})
	must(err)
	if job, err := s.ClaimCuration(ctx, p.ID, false, true); job != nil || err != nil {
		t.Fatal("legacy claim after API binding", job, err)
	}
	if job, err := s.ClaimCuration(guarded, other.ID, false, true); job != nil || err != nil {
		t.Fatal("cross-project claim", job, err)
	}
	job, err := s.ClaimCuration(guarded, p.ID, false, true)
	must(err)
	if job == nil {
		t.Fatal("missing direct API job")
	}
	if err := s.ReserveKnowledgeModelAttempt(guarded, b, "extract", job.ID, job.Lease); err != nil {
		t.Fatal(err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE knowledge_model_attempts SET status='failed',diagnostic='模型暂时不可用',attempts=3 WHERE lease=$1`, job.Lease)
	must(err)
	recent, err := s.KnowledgeModelSettings(ctx, p.ID)
	must(err)
	if recent["curator"].LastFailure != "模型暂时不可用（本任务已达尝试上限）" || recent["management"].LastFailure != "" {
		t.Fatal(recent)
	}
	out := Extraction{Session: "api", Facts: []KnowledgeFact{{Statement: "fact", Quote: "before"}}}
	if err = s.RecordExtraction(ctx, p.ID, job.ID, job.Lease, out); !errors.Is(err, ErrConflict) {
		t.Fatal("legacy result on bound role", err)
	}
	// The profile keeps its stable ID when the Owner rotates the connection.
	prev := m.Revision
	m.Revision = uuid.New()
	must(s.SaveChatModel(ctx, m, prev, func() error { return nil }))
	active, err := s.KnowledgeRoleModelCurrent(ctx, b)
	must(err)
	if active {
		t.Fatal("old catalog revision remains current")
	}
	if err = s.RecordExtraction(guarded, p.ID, job.ID, job.Lease, out); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted result for rotated model", err)
	}
	latest := b
	latest.ModelRevision = m.Revision
	if err = s.RecordExtraction(latest.WithContext(ctx), p.ID, job.ID, job.Lease, out); !errors.Is(err, ErrConflict) {
		t.Fatal("new binding relabeled an old provider response", err)
	}
	_, err = s.SaveMaterial(ctx, p.ID, "another", MaterialInput{Title: "second", Kind: "material", Source: "owner", Content: "before rotation"})
	must(err)
	newContext := latest.WithContext(ctx)
	second, err := s.ClaimCuration(newContext, p.ID, false, true)
	must(err)
	if second == nil || second.ID == job.ID {
		t.Fatal("expected different pending job", second)
	}
	must(s.ReserveKnowledgeModelAttempt(newContext, latest, "extract", second.ID, second.Lease))
	must(s.RecordExtraction(newContext, p.ID, second.ID, second.Lease, out))
	settings, err := s.KnowledgeModelSettings(ctx, p.ID)
	must(err)
	if settings["curator"].Revision != 1 || settings["curator"].ModelRevision.UUID != m.Revision || settings["curator"].LastFailure != "" {
		t.Fatal(settings)
	}
	settings, err = s.SetKnowledgeModel(ctx, p.ID, "management", 0, nil)
	must(err)
	if settings["management"].Revision != 1 || settings["management"].ModelID.Valid {
		t.Fatal("explicit stop did not persist", settings)
	}
	must(s.RemoveChatModel(ctx, m.ID, m.Revision))
	settings, err = s.KnowledgeModelSettings(ctx, p.ID)
	must(err)
	if settings["curator"].Revision != 2 || settings["curator"].ModelID.Valid {
		t.Fatal("deleted catalog model kept binding", settings)
	}
	if err = s.RecordExtraction(guarded, p.ID, job.ID, job.Lease, out); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted disabled model output", err)
	}
	var pending bool
	must(s.db.QueryRowContext(ctx, "SELECT extraction IS NULL FROM material_curations WHERE id=$1", job.ID).Scan(&pending))
	if !pending {
		t.Fatal("stale write changed knowledge")
	}
	bindings, err = s.ConfiguredKnowledgeRoleModels(ctx)
	must(err)
	if len(bindings) != 0 {
		t.Fatal(bindings)
	}
}

func TestKnowledgeModelConcurrentSettings(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := ChatModel{ID: uuid.New(), Revision: uuid.New(), Name: "api", Model: "api"}
	if err := s.SaveChatModel(ctx, m, uuid.Nil, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.SetKnowledgeModel(ctx, p.ID, "curator", 0, &m.ID); done <- err }()
	}
	a, b := <-done, <-done
	if !((a == nil && errors.Is(b, ErrConflict)) || (b == nil && errors.Is(a, ErrConflict))) {
		t.Fatalf("concurrent updates: %v / %v", a, b)
	}
	bindings, err := s.ConfiguredKnowledgeRoleModels(ctx)
	if err != nil || len(bindings) != 1 {
		t.Fatal(bindings, err)
	}
	binding := bindings[0]
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	allowed, err := knowledgeModelAllowed(binding.WithContext(ctx), tx, p.ID, "curator")
	if err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	// A successful recheck must protect the catalog and the binding until the
	// result commits, even when both transactions already read the same revision.
	for _, query := range []string{
		"SELECT 1 FROM chat_models WHERE id=$1 FOR UPDATE NOWAIT",
		"SELECT 1 FROM knowledge_model_settings WHERE model_id=$1 FOR UPDATE NOWAIT",
	} {
		var n int
		err := s.db.QueryRowContext(ctx, query, m.ID).Scan(&n)
		var dbErr interface{ SQLState() string }
		if !errors.As(err, &dbErr) || dbErr.SQLState() != "55P03" {
			t.Fatalf("missing output transaction lock on %s: %v", query, err)
		}
	}
}
