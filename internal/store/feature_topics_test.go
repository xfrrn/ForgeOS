package store

import (
	"context"
	"errors"
	"testing"

	"forgeos/internal/testdb"
	"github.com/google/uuid"
)

func TestFeatureChatIsolationAndReplay(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	profile := uuid.New()
	if _, err = db.ExecContext(ctx, "INSERT INTO chat_models(id,model,revision) VALUES($1,'pilot',$2)", profile, uuid.New()); err != nil {
		t.Fatal(err)
	}
	create := func(key uuid.UUID, title string) FeatureTopic {
		t.Helper()
		v, err := s.CreateFeature(ctx, project.ID, key, profile, title)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	key := uuid.New()
	a := create(key, "筛选")
	b := create(uuid.New(), "预览")
	if create(key, "筛选").ID != a.ID {
		t.Fatal("feature replay changed identity")
	}
	if _, err := s.Feature(ctx, other.ID, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross project lookup: %v", err)
	}
	message := uuid.NewString()
	first, err := s.SendFeatureChat(ctx, project.ID, a.ID, message, "restore filters", profile)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.SendFeatureChat(ctx, project.ID, a.ID, message, "restore filters", profile)
	if err != nil || first.ID != again.ID {
		t.Fatal("message replay", err)
	}
	if _, err = s.SendFeatureChat(ctx, project.ID, b.ID, message, "restore filters", profile); !errors.Is(err, ErrConflict) {
		t.Fatal("cross topic replay", err)
	}
	for _, id := range []int64{0, b.ID} {
		rows, err := s.FeatureHistory(ctx, project.ID, id, 0, 20)
		if err != nil || len(rows) != 0 {
			t.Fatal("leaked history", err)
		}
	}
	rows, err := s.FeatureHistory(ctx, project.ID, a.ID, 0, 20)
	if err != nil || len(rows) != 1 || rows[0].ID != first.ID {
		t.Fatal("missing durable history", err)
	}
}

func TestFeatureNativeScheduling(t *testing.T) {
	ctx := context.Background()
	s := New(testdb.Open(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	report := []FeatureClient{{Instance: "local", Backend: "codex", Model: "pilot"}}
	if err = s.ReportFeatureClients(ctx, project.ID, "node-a", report); err != nil {
		t.Fatal(err)
	}
	clients, err := s.FeatureClients(ctx, project.ID)
	if err != nil || len(clients) != 1 {
		t.Fatal(clients, err)
	}
	client := clients[0]
	topic, err := s.CreateFeature(ctx, project.ID, uuid.New(), client.ID, "native discussion")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.SendFeatureChat(ctx, project.ID, topic.ID, uuid.NewString(), "hello", client.ID)
	if err != nil || turn.NativeBackend != "codex" {
		t.Fatal(turn, err)
	}
	for _, other := range []string{"", "node-b"} {
		job, err := s.ClaimFeatureAwareChat(ctx, project.ID, true, other)
		if err != nil || job != nil {
			t.Fatal("unexpected claim", job, err)
		}
	}
	job, err := s.ClaimFeatureAwareChat(ctx, project.ID, true, "node-a")
	if err != nil || job == nil || job.Native == nil || job.Native.ID != client.ID || job.FeatureID != topic.ID {
		t.Fatal(job, err)
	}
	if err = s.ReportFeatureClients(ctx, project.ID, "node-a", nil); err != nil {
		t.Fatal(err)
	}
	_, err = s.SendFeatureChat(ctx, project.ID, topic.ID, uuid.NewString(), "offline", client.ID)
	if !errors.Is(err, ErrConflict) {
		t.Fatal("offline selection", err)
	}
}
