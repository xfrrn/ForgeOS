package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestQueueStartupBindsInventoryAndWorker(t *testing.T) {
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
	parent, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	head, err := s.Claim(ctx, "reader", root, false)
	if err != nil || head == nil {
		t.Fatal(err)
	}
	commit, blob := strings.Repeat("a", 40), strings.Repeat("b", 40)
	if err = s.Complete(ctx, head.ExecutionID, head.LeaseToken, Result{Commit: commit}); err != nil {
		t.Fatal(err)
	}
	files := json.RawMessage(`[{"path":"project.yaml","blob":"` + blob + `","local_environments":{"local-integrated":{"start_script":"up.sh","port":8000,"health_path":"/","timeout_seconds":10}}}]`)
	if _, err = s.SaveRepositoryInventory(ctx, parent.ID, head.ExecutionID, commit, "", files); err != nil {
		t.Fatal(err)
	}
	if _, err = s.QueueStartup(ctx, parent.ID, "local-mock", 0); !errors.Is(err, ErrConflict) {
		t.Fatal("undeclared environment", err)
	}
	var ids [4]int64
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := s.QueueStartup(ctx, parent.ID, "local-integrated", 0)
			if err != nil {
				t.Error(err)
			}
			ids[i] = id
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if id == 0 || id != ids[0] {
			t.Fatal("non-idempotent creation", ids)
		}
	}
	caps := WorkerCapabilities{SupportsResources: true, SupportsStartup: true, StartupIntegratedProject: p.ID}
	node := WorkerHeartbeat{Worker: "runner", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, ModelSlots: 1, CISlots: 1, Capabilities: caps}
	if err = s.RecordHeartbeat(ctx, node); err != nil {
		t.Fatal(err)
	}
	for _, c := range []WorkerCapabilities{{SupportsCodex: true}, {SupportsResources: true, SupportsStartup: true, StartupIntegratedProject: p.ID + 1}} {
		if j, err := s.ClaimForProjects(ctx, node.Worker, root, nil, c); err != nil || j != nil {
			t.Fatal("ineligible worker claimed task", err)
		}
	}
	if j, err := s.ClaimForProjects(ctx, node.Worker, root, []int64{p.ID + 1}, caps); err != nil || j != nil {
		t.Fatal("wrong grant", err)
	}
	j, err := s.ClaimForProjects(ctx, node.Worker, root, nil, caps)
	if err != nil || j == nil || !j.ManageResources || j.Startup.Commit != commit || j.Startup.ConfigBlob != blob {
		t.Fatal("missing pinned job", err)
	}
	if _, err = s.ReserveExecutionResource(ctx, j.ExecutionID, ResourceRequest{Lease: j.LeaseToken, Kind: "model", Step: 1}); !errors.Is(err, ErrConflict) {
		t.Fatal("environment task reserved model capacity", err)
	}
	log := LiveCILog{Run: "ci-run-" + strconv.FormatInt(j.ExecutionID, 10), Commit: commit, Check: "local-integrated", Bytes: 6, Text: "hello\n"}
	post := func(l LiveCILog, ok bool) {
		t.Helper()
		err := s.ObserveCILog(ctx, j.ExecutionID, CILogReport{Lease: j.LeaseToken, Log: l})
		if ok && err != nil || !ok && !errors.Is(err, ErrConflict) {
			t.Fatal(ok, err)
		}
	}
	if !j.ReportLiveCILog {
		t.Fatal("missing startup log capability")
	}
	post(log, false)
	if _, err = s.ReserveExecutionResource(ctx, j.ExecutionID, ResourceRequest{Lease: j.LeaseToken, Kind: "ci", Step: 1}); err != nil {
		t.Fatal(err)
	}
	post(log, true)
	post(log, true)
	for _, change := range []func(*LiveCILog){func(l *LiveCILog) { l.Commit = blob }, func(l *LiveCILog) { l.Check = "local-mock" }, func(l *LiveCILog) { l.Run = "ci-run-999" }, func(l *LiveCILog) { l.Index = 1 }, func(l *LiveCILog) { l.Text = "other\n" }} {
		bad := log
		change(&bad)
		post(bad, false)
	}
	if _, err = s.QueueStartup(ctx, parent.ID, "local-integrated", parent.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("unrelated retry accepted", err)
	}
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{Commit: commit}); !errors.Is(err, ErrConflict) {
		t.Fatal("missing receipt accepted", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	r := &EnvironmentRun{EnvironmentRequest: EnvironmentRequest{Repo: "project", Commit: commit, Environment: "local-integrated"}, ID: "execution-" + strconv.FormatInt(j.ExecutionID, 10), Worker: node.Worker, Started: now, Finished: now, ConfigBlob: blob, Config: &j.Startup.Config, ScriptBlob: commit, Image: "sha256:" + strings.Repeat("1", 64), Network: strings.Repeat("2", 64), Probes: 1, StatusCode: 204, Removed: true}
	result := Result{Commit: commit, EnvironmentRun: r}
	if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); !errors.Is(err, ErrConflict) {
		t.Fatal("discarded startup output", err)
	}
	for _, output := range []*EnvironmentOutput{{Bytes: 5, Text: "hello\n"}, {Bytes: 6, Text: "other\n"}, {Bytes: 64, Text: j.LeaseToken + "\n"}} {
		r.Output = output
		if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); !errors.Is(err, ErrConflict) {
			t.Fatal("invalid startup output", err)
		}
	}
	r.Output = &EnvironmentOutput{Bytes: 6, Text: "hello\n"}
	for _, bad := range []func(*EnvironmentRun){func(r *EnvironmentRun) { r.Commit = blob }, func(r *EnvironmentRun) { r.Removed = false }, func(r *EnvironmentRun) { r.Network = "none" }, func(r *EnvironmentRun) { r.Worker = "other" }} {
		copy := *r
		bad(&copy)
		err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{Commit: commit, EnvironmentRun: &copy})
		if !errors.Is(err, ErrConflict) {
			t.Fatal("mismatched receipt accepted", err)
		}
	}
	for range 2 {
		if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); err != nil {
			t.Fatal("completion replay", err)
		}
	}
	next, err := s.QueueStartup(ctx, parent.ID, "local-integrated", j.TaskID)
	if err != nil || next == j.TaskID {
		t.Fatal("missing retry", err)
	}
	if id, err := s.QueueStartup(ctx, parent.ID, "local-integrated", j.TaskID); err != nil || id != next {
		t.Fatal("duplicate retry", err)
	}
}
