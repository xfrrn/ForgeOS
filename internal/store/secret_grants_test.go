package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

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
