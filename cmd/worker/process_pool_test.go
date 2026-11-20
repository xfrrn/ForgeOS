package main

import (
	"forgeos/internal/store"
	"testing"
	"time"
)

func TestPoolBurstAndDrain(t *testing.T) {
	p := store.PoolLimits{Normal: 3, Maximum: 5}
	start := time.Now()
	var full time.Time
	for _, step := range []struct{ second, occupied, limit int }{
		{0, 0, 3}, {1, 3, 3}, {15, 3, 3}, {16, 3, 5}, {17, 5, 5}, {19, 2, 3}, {20, 3, 3}, {34, 3, 3}, {35, 3, 5},
	} {
		if got := poolCapacity(p, step.occupied, start.Add(time.Duration(step.second)*time.Second), &full); got != step.limit {
			t.Fatalf("at %ds: got %d want %d", step.second, got, step.limit)
		}
	}
	if got := poolCapacity(store.PoolLimits{Normal: 1, Maximum: 1}, 4, start.Add(time.Minute), &full); got != 1 {
		t.Fatal("lowering hard limit must drain")
	}
}
