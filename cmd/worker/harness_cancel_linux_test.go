package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNativeTurnCancellationStopsLinuxTool(t *testing.T) {
	for _, name := range []string{"codex", "claude"} {
		t.Run(name, func(t *testing.T) {
			work := t.TempDir()
			bin := filepath.Join(work, "bounded-harness")
			if err := os.WriteFile(bin, []byte("#!/bin/sh\nsh -c 'echo $$ >tool.pid; exec sleep 8' &\nwait\n"), 0700); err != nil {
				t.Fatal(err)
			}
			var adapter harnessAdapter = codexCLI{bin: bin, sandbox: "workspace-write"}
			if name == "claude" {
				adapter = claudeCLI{bin: bin}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := adapter.runTurn(ctx, processLog{}, work, "test", "")
				done <- err
			}()
			var pid int
			for pid < 1 && ctx.Err() == nil {
				data, err := os.ReadFile(filepath.Join(work, "tool.pid"))
				if err == nil {
					pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
				}
				time.Sleep(10 * time.Millisecond)
			}
			if pid < 1 {
				t.Fatal("tool did not start")
			}
			cancel()
			if err := <-done; err == nil {
				t.Fatal("cancelled turn succeeded")
			}
			limit := time.Now().Add(time.Second)
			for {
				stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
				if errors.Is(err, os.ErrNotExist) {
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				_, state, ok := strings.Cut(string(stat), ") ")
				if ok && strings.HasPrefix(state, "Z ") {
					return
				}
				if time.Now().After(limit) {
					t.Fatalf("tool PID %d still running after turn cancellation", pid)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
