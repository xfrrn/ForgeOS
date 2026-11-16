package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedWorkerCannotExchangeProjectGrants(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	const worker = "shared"
	if err := s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: worker, OS: "linux", Arch: "amd64", CPUs: 2, Slots: 2}); err != nil {
		t.Fatal(err)
	}
	type scope struct {
		meta  SecretMeta
		job   *Job
		grant int64
	}
	var projects [2]scope
	for i := range projects {
		p, err := s.CreateProject(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateTask(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		j, err := s.ClaimForProjects(ctx, worker, t.TempDir(), []int64{p.ID}, WorkerCapabilities{})
		if err != nil || j == nil {
			t.Fatal("claim", err)
		}
		m, err := s.SetSecretMeta(ctx, SecretMeta{ProjectID: p.ID, Environment: "ci-test", Name: "receipt", Risk: "S1"})
		if err != nil {
			t.Fatal(err)
		}
		g := SecretGrantInput{ExecutionID: j.ExecutionID, Principal: worker, Environment: m.Environment, Name: m.Name, Version: 1, Purpose: "verify", ExpiresAt: time.Now().Add(time.Minute), RequestKey: "approval"}
		doc, err := s.ApproveSecretGrant(ctx, m, g)
		var grant struct{ ID int64 }
		if err != nil || json.Unmarshal(doc, &grant) != nil || grant.ID == 0 {
			t.Fatal("approve", err)
		}
		projects[i] = scope{m, j, grant.ID}
	}
	for i, own := range projects {
		other := projects[1-i]
		u := SecretUse{ExecutionID: own.job.ExecutionID, Environment: own.meta.Environment, Name: own.meta.Name, Purpose: "verify", RequestKey: "use", Lease: own.job.LeaseToken}
		calls := 0
		wrap := func(m SecretMeta, version int, until time.Time) (time.Time, error) {
			calls++
			if m.ProjectID != own.meta.ProjectID || version != 1 {
				t.Fatal("incorrect OpenBao path or version", m, version)
			}
			return until.Add(-time.Second), nil
		}
		if err := s.UseSecret(ctx, own.meta.ProjectID, other.grant, worker, u, wrap); !errors.Is(err, ErrConflict) {
			t.Fatal("foreign grant", err)
		}
		if err := s.UseSecret(ctx, other.meta.ProjectID, other.grant, worker, u, wrap); !errors.Is(err, ErrConflict) || calls != 0 {
			t.Fatal("foreign execution", err, calls)
		}
		if err := s.UseSecret(ctx, own.meta.ProjectID, own.grant, worker, u, wrap); err != nil || calls != 1 {
			t.Fatal("own grant", err, calls)
		}
		if _, err := s.SecretGrantReceipt(ctx, other.meta.ProjectID, own.grant); !errors.Is(err, ErrNotFound) {
			t.Fatal("foreign receipt", err)
		}
	}
}

func TestSecretUseSerializesAndBindsMetadata(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateTask(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	j, err := s.Claim(ctx, "node", t.TempDir())
	if err != nil || j == nil {
		t.Fatal(err)
	}
	m, err := s.SetSecretMeta(ctx, SecretMeta{ProjectID: p.ID, Environment: "ci-test", Name: "fixture", Risk: "S1"})
	if err != nil {
		t.Fatal(err)
	}
	one := 1
	g := SecretGrantInput{ExecutionID: j.ExecutionID, Principal: "node", Environment: m.Environment, Name: m.Name, Version: 2, Purpose: "fixture", ExpiresAt: time.Now().Add(time.Minute), MaxUses: &one, RequestKey: "approval"}
	approve := func(key string) int64 {
		t.Helper()
		g.RequestKey = key
		doc, err := s.ApproveSecretGrant(ctx, m, g)
		var out struct{ ID int64 }
		if err != nil || json.Unmarshal(doc, &out) != nil || out.ID <= 0 {
			t.Fatal(string(doc), err)
		}
		return out.ID
	}
	id := approve("first")
	u := SecretUse{ExecutionID: j.ExecutionID, Environment: m.Environment, Name: m.Name, Purpose: g.Purpose, Lease: j.LeaseToken}
	var calls atomic.Int32
	wrap := func(meta SecretMeta, v int, until time.Time) (time.Time, error) {
		calls.Add(1)
		if meta.ProjectID != p.ID || v != 2 || meta.Risk != "S1" {
			return time.Time{}, errors.New("incorrect authorized scope")
		}
		return until.Add(-time.Second), nil
	}
	results := make(chan error, 2)
	for _, key := range []string{"left", "right"} {
		go func(key string) {
			attempt := u
			attempt.RequestKey = key
			results <- s.UseSecret(ctx, p.ID, id, "node", attempt, wrap)
		}(key)
	}
	success := 0
	for range 2 {
		if err := <-results; err == nil {
			success++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 || calls.Load() != 1 {
		t.Fatal(success, calls.Load())
	}
	second := approve("second")
	// Even reverting a risk edit must not resurrect the approval made against an older metadata version.
	for _, risk := range []string{"S3", "S1"} {
		m.Risk = risk
		if _, err = s.SetSecretMeta(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	u.RequestKey = "stale"
	if err = s.UseSecret(ctx, p.ID, second, "node", u, wrap); !errors.Is(err, ErrConflict) || calls.Load() != 1 {
		t.Fatal(err, calls.Load())
	}
}
