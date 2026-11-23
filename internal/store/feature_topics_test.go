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

func TestFeatureClientOperations(t *testing.T) {
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
	if _, err = db.ExecContext(ctx, `INSERT INTO worker_nodes(worker,os,arch,cpus,slots,capabilities,model_slots) VALUES('host','windows','amd64',1,2,'[]',1)`); err != nil {
		t.Fatal(err)
	}
	clients := []FeatureClient{{Instance: "local", Backend: "codex", Model: "test", Status: "ready"}}
	report := func(gen int64) {
		t.Helper()
		if err := s.RecordFeatureProbe(ctx, project.ID, "chat", "host", clients, gen, false); err != nil {
			t.Fatal(err)
		}
	}
	report(0)
	live, err := s.FeatureClients(ctx, project.ID)
	if err != nil || len(live) != 1 {
		t.Fatal(live, err)
	}
	client := live[0].ID
	if err = s.RequestFeatureProbe(ctx, project.ID, client); err != nil {
		t.Fatal(err)
	}
	gen, err := s.PendingFeatureProbe(ctx, project.ID, "chat")
	if err != nil || gen != 1 {
		t.Fatal(gen, err)
	}
	clients[0].Status = "auth"
	report(gen)
	health, err := s.ClientHealth(ctx, project.ID)
	if err != nil || len(health) != 1 || health[0].Pending || health[0].Status != "auth" || health[0].CheckedAt == nil {
		t.Fatal(health, err)
	}
	live, err = s.FeatureClients(ctx, project.ID)
	if err != nil || len(live) != 0 {
		t.Fatal(live, err)
	}
	clients[0].Status = "ready"
	report(0)
	create := func() FeatureTopic {
		t.Helper()
		f, err := s.CreateFeature(ctx, project.ID, uuid.New(), client, "test")
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	send := func(f FeatureTopic) ChatTurn {
		t.Helper()
		r, err := s.SendFeatureChat(ctx, project.ID, f.ID, uuid.NewString(), "hello", client)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	claim := func() *ChatJob {
		t.Helper()
		j, err := s.ClaimFeatureAwareChat(ctx, project.ID, true, "chat")
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	a, b := create(), create()
	first := send(a)
	second := send(b)
	j := claim()
	if j == nil || j.ID != first.ID {
		t.Fatal(j)
	}
	if err = s.SetModelBudgets(ctx, []ModelBudget{{Name: "lab", Harness: "codex", Workers: []string{"host"}, Slots: 1}}); err != nil {
		t.Fatal(err)
	}
	var reservations int
	if err = db.QueryRowContext(ctx, "SELECT reserved FROM model_budget_usage WHERE name='lab'").Scan(&reservations); err != nil || reservations != 1 {
		t.Fatal("configured budget omitted running discussion", reservations, err)
	}
	if claim() != nil {
		t.Fatal("machine model slot overcommitted")
	}
	if err = s.CancelFeatureTurn(ctx, project.ID, b.ID, first.ID); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = s.CancelFeatureTurn(ctx, project.ID, a.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	stop, err := s.FeatureCancelRequested(ctx, project.ID, j.ID, j.Lease)
	if !stop || err != nil {
		t.Fatal(stop, err)
	}
	if err = s.CompleteChat(ctx, project.ID, j.ID, j.Lease, "late reply", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	row, err := s.ChatTurn(ctx, project.ID, j.ID)
	if err != nil || row.Answer != "" || row.Error != FeatureCancelled {
		t.Fatal(row, err)
	}
	j = claim()
	if j == nil || j.ID != second.ID {
		t.Fatal(j)
	}
	if err = s.CompleteChat(ctx, project.ID, j.ID, j.Lease, "", FeatureSessionMissing, nil, nil); err != nil {
		t.Fatal(err)
	}
	recovery, err := s.RecoverFeatureTurn(ctx, project.ID, b.ID, j.ID)
	if err != nil || recovery.RecoveryOf != j.ID {
		t.Fatal(recovery, err)
	}
	again, err := s.RecoverFeatureTurn(ctx, project.ID, b.ID, j.ID)
	if err != nil || again.ID != recovery.ID {
		t.Fatal(again, err)
	}
	j = claim()
	if j == nil || j.Generation != recovery.ID || j.ExpectSession {
		t.Fatal(j)
	}
}
