package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"forgeos/internal/store"
)

type executionResources struct {
	worker *worker
	job    store.Job
	step   int64
}

func (b *executionResources) request(ctx context.Context, kind string, release bool) error {
	path := fmt.Sprintf("/internal/executions/%d/resource", b.job.ExecutionID)
	input := store.ResourceRequest{Lease: b.job.LeaseToken, Step: b.step, Kind: kind, Release: release}
	for ctx.Err() == nil {
		status, err := b.worker.post(ctx, path, input, nil)
		if err == nil && status == http.StatusNoContent {
			return nil
		}
		if !temporary(status) {
			return fmt.Errorf("%s resource request rejected (HTTP %d)", kind, status)
		}
		wait(ctx)
	}
	return ctx.Err()
}

func withResource[T any](ctx context.Context, b *executionResources, kind string, run func() (T, error)) (T, error) {
	if !b.job.ManageResources {
		return run()
	}
	b.step++
	if err := b.request(ctx, kind, false); err != nil {
		var empty T
		return empty, err
	}
	result, err := run()
	return result, errors.Join(err, b.request(ctx, kind, true))
}
