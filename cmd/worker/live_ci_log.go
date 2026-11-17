package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

type ciLogFile struct {
	dir, run, check string
	index           int
}

var secretVariable = regexp.MustCompile(`(?i)(token|password|passwd|secret|private_key|api_key|database_url)`)

func (w *worker) watchCILog(parent context.Context, job store.Job, commit string) (func(ciLogFile), func()) {
	ctx, cancel := context.WithCancel(parent)
	var current atomic.Pointer[ciLogFile]
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		path := fmt.Sprintf("/internal/executions/%d/ci-log", job.ExecutionID)
		secrets := []string{job.LeaseToken}
		for _, entry := range os.Environ() {
			key, value, _ := strings.Cut(entry, "=")
			if secretVariable.MatchString(key) && value != "" {
				secrets = append(secrets, value)
			}
		}
		var sent *ciLogFile
		size := -1
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			file := current.Load()
			if file == nil {
				continue
			}
			name := file.check + ".log"
			info, err := os.Lstat(filepath.Join(file.dir, name))
			if err != nil || (file == sent && info.Size() == int64(size)) {
				continue
			}
			data, err := archivedFile(file.dir, name, int64(validation.MaxLogBytes))
			if err != nil {
				continue
			}
			token, err := w.credential()
			if err != nil {
				continue
			}
			sample := store.LiveCILog{Run: file.run, Index: file.index, Check: file.check, Commit: commit, Bytes: len(data), Text: validation.DisplayLogTail(data, append(secrets, token))}
			call, stop := context.WithTimeout(ctx, 3*time.Second)
			status, err := w.post(call, path, store.CILogReport{Lease: job.LeaseToken, Log: sample}, nil)
			stop()
			if err == nil && status == 204 {
				sent, size = file, len(data)
			}
		}
	}()
	return func(file ciLogFile) { current.Store(&file) }, func() { cancel(); <-done }
}
