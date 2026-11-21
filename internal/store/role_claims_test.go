package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRoleClaimAtomicReplay(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "claim-replay")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"first", "second"} {
		if _, err = s.SaveMaterial(ctx, p.ID, key, MaterialInput{Kind: "material", Title: key, Source: "observed", Content: "Offset 274."}); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	claim := func(ctx context.Context) (int, []byte, error) {
		calls.Add(1)
		j, e := s.ClaimCuration(ctx, p.ID, false, true)
		if e != nil {
			return 0, nil, e
		}
		if j == nil {
			return 204, nil, nil
		}
		j.CaptureProcess = true
		b, e := json.Marshal(j)
		return 200, b, e
	}
	key := strings.Repeat("a", 32)
	var wg sync.WaitGroup
	results := make(chan []byte, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, b, e := s.RoleClaim(ctx, p.ID, "curator", "node", key, []byte("same request"), claim)
			if e != nil || status != 200 {
				t.Error(status, e)
				return
			}
			results <- b
		}()
	}
	wg.Wait()
	close(results)
	var first []byte
	for b := range results {
		if first == nil {
			first = b
		}
		if !bytes.Equal(first, b) {
			t.Fatal("different lease or frozen response")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate allocation", calls.Load())
	}
	status, b, err := New(s.db).RoleClaim(ctx, p.ID, "curator", "node", key, []byte("same request"), claim)
	if err != nil || status != 200 || !bytes.Equal(first, b) || calls.Load() != 1 {
		t.Fatal("restart did not replay", status, err)
	}
	if _, _, err = s.RoleClaim(ctx, p.ID, "curator", "node", key, []byte("changed capabilities"), claim); !errors.Is(err, ErrConflict) {
		t.Fatal("changed request reused key", err)
	}
	// Failure after allocating the next job must roll the allocation back with its receipt.
	_, _, err = s.RoleClaim(ctx, p.ID, "curator", "node", strings.Repeat("b", 32), nil, func(ctx context.Context) (int, []byte, error) {
		status, body, e := claim(ctx)
		if e != nil || status != 200 {
			return status, body, e
		}
		return 0, nil, errors.New("lost before receipt commit")
	})
	if err == nil {
		t.Fatal("injected failure committed")
	}
	status, b, err = s.RoleClaim(ctx, p.ID, "curator", "node", strings.Repeat("b", 32), nil, claim)
	if err != nil || status != 200 || bytes.Equal(first, b) {
		t.Fatal("rollback lost available job", status, err)
	}
	var job CurationJob
	if err = json.Unmarshal(first, &job); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordExtraction(ctx, p.ID, job.ID, job.Lease, Extraction{Session: "curator", Facts: []KnowledgeFact{{Statement: "Offset observed", Quote: "274"}}}); err != nil {
		t.Fatal(err)
	}
	// Management tries multiple sources inside the same transaction before selecting a job.
	status, b, err = s.RoleClaim(ctx, p.ID, "management", "node", key, nil, func(ctx context.Context) (int, []byte, error) {
		j, e := s.ClaimManagement(ctx, p.ID, true, true)
		if e != nil || j != nil {
			return 0, nil, errors.New("unexpected plan")
		}
		c, e := s.ClaimCuration(ctx, p.ID, true, true)
		if e != nil {
			return 0, nil, e
		}
		if c == nil {
			return 0, nil, errors.New("missing knowledge review")
		}
		raw, e := json.Marshal(ManagementJob{Curation: c})
		return 200, raw, e
	})
	if err != nil || status != 200 || !bytes.Contains(b, []byte("curation")) {
		t.Fatal("shared fallback transaction", status, err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM role_claim_receipts").Scan(&count); err != nil || count != 3 {
		t.Fatal("receipt count", count, err)
	}
	// Previously allocated inputs should also stop replaying when an Owner
	// disables either role. An old request key never allocates another job.
	for _, role := range []string{"curator", "management"} {
		if _, err = s.SetKnowledgeModel(ctx, p.ID, role, 0, nil); err != nil {
			t.Fatal(err)
		}
		var request []byte
		if role == "curator" {
			request = []byte("same request")
		}
		status, body, err := s.RoleClaim(ctx, p.ID, role, "node", key, request, func(context.Context) (int, []byte, error) {
			t.Fatal("old receipt allocated a new job")
			return 0, nil, nil
		})
		if err != nil || status != 204 || len(body) != 0 {
			t.Fatalf("disabled %s replayed old input: %d %v", role, status, err)
		}
	}
}

func TestRoleClaimEmptyPollCanLaterAllocate(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "empty-poll")
	if err != nil {
		t.Fatal(err)
	}
	claim := func(ctx context.Context) (int, []byte, error) {
		j, e := s.ClaimCuration(ctx, p.ID, false, true)
		if e != nil {
			return 0, nil, e
		}
		if j == nil {
			return 204, nil, nil
		}
		raw, e := json.Marshal(j)
		return 200, raw, e
	}
	key := strings.Repeat("c", 32)
	if status, _, e := s.RoleClaim(ctx, p.ID, "curator", "node", key, nil, claim); e != nil || status != 204 {
		t.Fatal(status, e)
	}
	if _, err = s.SaveMaterial(ctx, p.ID, "new", MaterialInput{Kind: "material", Title: "New", Source: "observed", Content: "Offset 274."}); err != nil {
		t.Fatal(err)
	}
	if status, _, e := s.RoleClaim(ctx, p.ID, "curator", "node", key, nil, claim); e != nil || status != 200 {
		t.Fatal(status, e)
	}
}

func TestRoleClaimEmptyCandidateReleasesModelLock(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "empty-budget")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetModelBudgets(ctx, []ModelBudget{{Name: "manager", Harness: "codex", Slots: 1, AgentRoles: []ModelBudgetRole{{Project: p.ID, Role: "plan_review"}}}}); err != nil {
		t.Fatal(err)
	}
	_, _, err = s.RoleClaim(ctx, p.ID, "management", "node", strings.Repeat("d", 32), nil, func(ctx context.Context) (int, []byte, error) {
		j, e := s.ClaimManagement(ctx, p.ID, true, true)
		if e != nil {
			return 0, nil, e
		}
		if j != nil {
			return 0, nil, errors.New("unexpected plan")
		}
		other, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return 0, nil, e
		}
		defer other.Rollback()
		_, e = other.ExecContext(ctx, "SELECT name FROM model_budgets WHERE name='manager' FOR UPDATE NOWAIT")
		return 204, nil, e
	})
	if err != nil {
		t.Fatal("empty fallback held a model lock", err)
	}
}
