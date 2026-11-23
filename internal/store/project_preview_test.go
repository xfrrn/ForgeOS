package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"forgeos/internal/testdb"
	"github.com/google/uuid"
)

func TestProjectPreviewReceiptsAndReplay(t *testing.T) {
	ctx := context.Background()
	s := New(testdb.Open(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	worker := "node"
	caps := WorkerCapabilities{SupportsProjectPreview: true}
	must(s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: worker, OS: "windows", Arch: "amd64", CPUs: 2, Slots: 1, Capabilities: caps}))
	first, duplicate := uuid.New(), uuid.New()
	must(s.QueueProjectPreview(ctx, project.ID, worker, "start", first))
	must(s.QueueProjectPreview(ctx, project.ID, worker, "start", duplicate))
	initial, err := s.ProjectPreview(ctx, project.ID)
	must(err)
	if initial.Phase != "READY" || initial.State != "stopped" {
		t.Fatalf("expected a queued start: %+v", initial)
	}
	claim := func() *Job {
		t.Helper()
		j, err := s.ClaimForProjects(ctx, worker, t.TempDir(), nil, caps)
		must(err)
		if j == nil || j.Preview == nil || j.Preview.ProjectID != project.ID {
			t.Fatalf("expected a project preview job: %+v", j)
		}
		return j
	}
	start := claim()
	if start.TaskID != initial.TaskID {
		t.Fatalf("duplicate start allocated another job: %+v", start)
	}
	if err := s.QueueProjectPreview(ctx, project.ID, worker, "stop", uuid.New()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stop must wait until start settles: %v", err)
	}
	report := &ProjectPreviewReport{OperationID: start.TaskID, ProjectID: project.ID + 1, Action: "start",
		Outcome: "ok", State: "running", Instance: uuid.NewString(), Commit: strings.Repeat("a", 40), HTTPStatus: 200}
	result := Result{ProjectPreview: report, PreviewPort: 9001}
	if err := s.Complete(ctx, start.ExecutionID, start.LeaseToken, result); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-project report was accepted: %v", err)
	}
	report.ProjectID = project.ID
	must(s.Complete(ctx, start.ExecutionID, start.LeaseToken, result))
	route, err := s.ProjectPreviewTarget(ctx, initial.Key)
	must(err)
	if route.Port != 9001 || route.Instance != report.Instance || route.Secret != start.Preview.Secret {
		t.Fatalf("wrong preview target: port=%d", route.Port)
	}
	must(s.QueueProjectPreview(ctx, project.ID, worker, "stop", uuid.New()))
	stop := claim()
	must(s.Complete(ctx, stop.ExecutionID, stop.LeaseToken, Result{ProjectPreview: &ProjectPreviewReport{
		OperationID: stop.TaskID, ProjectID: project.ID, Action: "stop", Outcome: "ok", State: "stopped"}}))
	must(s.QueueProjectPreview(ctx, project.ID, worker, "start", duplicate))
	stopped, err := s.ProjectPreview(ctx, project.ID)
	must(err)
	if stopped.State != "stopped" || stopped.TaskID != stop.TaskID || stopped.Key != initial.Key {
		t.Fatalf("replaying an older request changed the active state: %+v", stopped)
	}
	if _, err := s.ProjectPreviewTarget(ctx, stopped.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stopped preview still had a route: %v", err)
	}
}
