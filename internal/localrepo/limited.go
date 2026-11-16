package localrepo

import (
	"context"
	"io"
)

// LimitedGit bounds stdout and stops Git as soon as the caller's size limit is exceeded.
func LimitedGit(ctx context.Context, repo string, limit int, args ...string) ([]byte, bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := GitCommand(ctx, repo, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err = cmd.Start(); err != nil {
		return nil, false, err
	}
	output, err := io.ReadAll(io.LimitReader(stdout, int64(limit)+1))
	truncated := len(output) > limit
	if truncated || err != nil {
		cancel()
	}
	exit := cmd.Wait()
	if err != nil {
		return nil, false, err
	}
	if truncated {
		return output[:limit], true, nil
	}
	return output, false, exit
}
