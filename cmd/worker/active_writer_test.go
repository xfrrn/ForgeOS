package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

func TestInterruptedWriterHistory(t *testing.T) {
	q := parkedCheckpoint{Session: "original", Active: &activeWriterTurn{NativeTurn: store.NativeTurn{Turn: "first"}}}
	q.Job.Workspace = t.TempDir()
	for _, tc := range []struct {
		session, cwd, turns string
		want                bool
	}{
		{"original", q.Job.Workspace, `[{"id":"first","status":"interrupted"}]`, true},
		{"elsewhere", q.Job.Workspace, `[{"id":"first","status":"interrupted"}]`, false},
		{"original", t.TempDir(), `[{"id":"first","status":"interrupted"}]`, false},
		{"original", q.Job.Workspace, `[]`, false},
		{"original", q.Job.Workspace, `[{"id":"first","status":"completed"}]`, false},
		{"original", q.Job.Workspace, `[{"id":"first","status":"interrupted"},{"id":"next","status":"interrupted"}]`, false},
	} {
		var h inputHistory
		h.Thread.ID, h.Thread.Cwd = tc.session, tc.cwd
		if err := json.Unmarshal([]byte(tc.turns), &h.Thread.Turns); err != nil {
			t.Fatal(err)
		}
		if h.latestWriter(&q, "interrupted") != tc.want {
			t.Errorf("session %s cwd %s turns %s: want %t", tc.session, tc.cwd, tc.turns, tc.want)
		}
	}
}

func TestCompletedWriterExitReceipt(t *testing.T) {
	if !supportsWriterProcessTree {
		t.Skip("requires a supported Worker platform")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	w, job, _ := fixture(t)
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var err error
	w.journal, err = w.openJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	w.codexAppServer, w.codexVersion, job.RecordAppTurn = true, "test", true
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	w.api, w.client = server.URL, server.Client()
	for _, tc := range []struct {
		arg string
		ok  bool
	}{{"--version", true}, {"--invalid-forgeos-test", false}} {
		p := w.processLog(ctx, job, "codex", w.codexVersion)
		cmd := exec.CommandContext(ctx, "git", tc.arg)
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := p.start(cmd, out); err != nil {
			t.Fatal(err)
		}
		if exit, journal := p.wait(cmd); (exit == nil) != tc.ok || journal != nil {
			t.Fatal(exit, journal)
		}
		q := parkedCheckpoint{Job: job, Session: "thread", Version: w.codexVersion, ReturnedTip: "saved",
			Active: &activeWriterTurn{NativeTurn: store.NativeTurn{Instance: p.id, Session: "thread", Turn: "turn"}, Deadline: time.Now().Add(time.Minute)}}
		if err := p.session(q.Session); err != nil {
			t.Fatal(err)
		}
		if err := p.appTurn(q.Active.Turn); err != nil {
			t.Fatal(err)
		}
		if err := w.waitForWriterExit(ctx, &q); (err == nil) != tc.ok || errors.Is(err, errTurnFailed) == tc.ok {
			t.Fatal(tc.arg, err)
		}
		if _, err := w.journal.Exec("UPDATE harness_processes SET exit_code=-1 WHERE instance=?", p.id); err != nil {
			t.Fatal(err)
		}
		if err := w.waitForWriterExit(ctx, &q); err == nil || errors.Is(err, errTurnFailed) {
			t.Fatal("unknown exit became a definite outcome", err)
		}
		q.Active.Deadline = time.Now().Add(-time.Second)
		if err := w.waitForWriterExit(ctx, &q); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("unknown exit outlasted the execution budget", err)
		}
		q.Active.Deadline = time.Now().Add(100 * time.Millisecond)
		recovery := checkpointRecovery{waiting: true, leaveUnsettled: true}
		if err := w.recoverActiveWriter(ctx, &executionResources{recovery: &recovery}, &q, ""); !errors.Is(err, context.DeadlineExceeded) || recovery.leaveUnsettled || recovery.waiting {
			t.Fatal("unconfirmed writer did not settle at its deadline", err, recovery)
		}
	}
}

func TestPendingWriterStartHistory(t *testing.T) {
	const key = "42d015ea-b797-4f3f-a58f-daa7e5ffe334"
	const item = `{"type":"userMessage","id":"message","clientId":"` + key + `","content":[{"type":"text","text":"Exact prompt"}]}`
	digest := sha256.Sum256([]byte("Exact prompt"))
	q := parkedCheckpoint{Session: "original", Active: &activeWriterTurn{RequestID: key, PromptHash: digest[:]}}
	q.Job.Workspace = t.TempDir()
	latest := `{"id":"latest","items":[` + item + `]}`
	for _, tc := range []struct{ turns, want string }{
		{latest, "latest"},
		{strings.Replace(latest, "Exact prompt", "Other prompt", 1), ""},
		{strings.Replace(latest, key, "wrong", 1), ""},
		{strings.Replace(latest, item, item+","+item, 1), ""},
		{strings.Replace(latest, item, `{"type":"userMessage","clientId":"earlier"},`+item, 1), ""},
		{strings.Replace(latest, "latest", "earlier", 1) + "," + latest, ""},
		{latest + `,{"id":"next","items":[]}`, ""},
	} {
		var h inputHistory
		h.Thread.ID, h.Thread.Cwd = q.Session, q.Job.Workspace
		if err := json.Unmarshal([]byte("["+tc.turns+"]"), &h.Thread.Turns); err != nil {
			t.Fatal(err)
		}
		if got := h.writerStart(&q); got != tc.want {
			t.Errorf("resolved %s, want %s for %s", got, tc.want, tc.turns)
		}
		h.Thread.Cwd = t.TempDir()
		if h.writerStart(&q) != "" {
			t.Fatal("accepted another workspace")
		}
	}
}

func TestWriterProcessTree(t *testing.T) {
	switch os.Getenv("FORGEOS_TEST_WRITER_CHILD") {
	case "tool":
		time.Sleep(30 * time.Second)
		return
	case "native":
		if _, err := bufio.NewReader(os.Stdin).ReadByte(); err != nil {
			t.Fatal(err)
		}
		tool := exec.Command(os.Args[0], "-test.run=^TestWriterProcessTree$")
		tool.Env = append(os.Environ(), "FORGEOS_TEST_WRITER_CHILD=tool")
		if err := tool.Start(); err != nil {
			t.Fatal(err)
		}
		fmt.Println(tool.Process.Pid)
		time.Sleep(30 * time.Second)
		return
	}
	if !supportsWriterProcessTree {
		t.Skip("unsupported worker OS")
	}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWriterProcessTree$")
	cmd.Env = append(os.Environ(), "FORGEOS_TEST_WRITER_CHILD=native")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	w, job, _ := fixture(t)
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w.journal, err = w.openJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	w.codexVersion, w.codexAppServer, job.RecordAppTurn = "test", true, true
	p := w.processLog(ctx, job, "codex", "test")
	if err := p.start(cmd, out); err != nil {
		t.Fatal(err)
	}
	defer cmd.Cancel()
	if err := trackWriterProcess(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	in.Close()
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Release()
	defer tool.Kill()
	q := parkedCheckpoint{Job: job, Session: "thread", Version: "test", Active: &activeWriterTurn{NativeTurn: store.NativeTurn{Instance: p.id, Session: "thread", Turn: "turn-1"}}}
	if err := p.session(q.Session); err != nil {
		t.Fatal(err)
	}
	startup, stopStartup := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stopStartup()
	if err := w.finishWriterStartup(startup, &q); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("live startup was allowed to overlap: %v", err)
	}
	if err := p.appTurn(q.Active.Turn); err != nil {
		t.Fatal(err)
	}
	if err := w.finishWriterStartup(ctx, &q); err == nil {
		t.Fatal("treated a registered model turn as an unstarted writer")
	}
	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if err := w.waitForWriterExit(short, &q); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	q.Version = "changed"
	if err := w.waitForWriterExit(ctx, &q); err == nil {
		t.Fatal("accepted different version")
	}
	q.Version = "test"
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if runtime.GOOS == "linux" {
		if alive, err := writerTreeAlive(cmd.Process.Pid); err != nil || !alive {
			t.Fatal("lost surviving tool", err)
		}
		return
	}
	end := time.Now().Add(5 * time.Second)
	for {
		alive, err := roleProcessAlive(pid)
		if err != nil {
			t.Fatal(err)
		}
		if !alive {
			break
		}
		if time.Now().After(end) {
			t.Fatal("tool survived native process")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := w.waitForWriterExit(ctx, &q); err != nil {
		t.Fatal(err)
	}
	var code int
	if err := w.journal.QueryRow("SELECT exit_code FROM harness_processes WHERE instance=?", p.id).Scan(&code); err != nil || code != -1 {
		t.Fatal(code, err)
	}
}
