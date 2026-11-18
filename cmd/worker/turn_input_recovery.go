package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"time"

	"forgeos/internal/store"
)

type inputHistory struct {
	Thread struct {
		ID, Cwd string
		Turns   []struct {
			ID, Status string
			Items      []struct {
				Type, ID, ClientID string
				Content            []struct{ Type, Text string }
			}
		}
	}
}

func (h inputHistory) matchesThread(workspace, session string) bool {
	if h.Thread.ID != session {
		return false
	}
	expected, err := os.Stat(workspace)
	actual, statErr := os.Stat(h.Thread.Cwd)
	return err == nil && statErr == nil && os.SameFile(expected, actual)
}

func (h inputHistory) confirms(workspace string, poll store.TurnInputPoll, digest []byte) bool {
	if !h.matchesThread(workspace, poll.Session) {
		return false
	}
	turns, messages := 0, 0
	for _, turn := range h.Thread.Turns {
		if turn.ID != poll.Turn {
			continue
		}
		turns++
		for _, item := range turn.Items {
			if item.Type != "userMessage" || item.ClientID != poll.Key {
				continue
			}
			if item.ID == "" || len(item.Content) != 1 || item.Content[0].Type != "text" {
				return false
			}
			sum := sha256.Sum256([]byte(item.Content[0].Text))
			if !bytes.Equal(sum[:], digest) {
				return false
			}
			messages++
		}
	}
	return turns == 1 && messages == 1
}

// thread/read inspects the rollout without resuming a session or starting a model turn.
func (c codexCLI) readInputHistory(ctx context.Context, workspace, session string) (inputHistory, error) {
	var history inputHistory
	cmd := exec.CommandContext(ctx, c.bin, c.appServerArgs(workspace)...)
	cmd.Dir, cmd.Env = workspace, harnessEnv()
	in, err := cmd.StdinPipe()
	if err != nil {
		return history, err
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		return history, err
	}
	defer out.Close()
	if err := cmd.Start(); err != nil {
		return history, err
	}
	s := codexServer{input: json.NewEncoder(in), scanner: bufio.NewScanner(out)}
	s.scanner.Buffer(make([]byte, 16<<10), 4<<20)
	err = s.initialize()
	if err == nil {
		err = s.call("thread/read", map[string]any{"threadId": session, "includeTurns": true}, &history)
	}
	in.Close()
	out.Close()
	return history, errors.Join(err, cmd.Wait())
}

func (w *worker) reconcileNativeInputs(ctx context.Context) {
	if !w.codexAppServer {
		return
	}
	var last, limit int64
	if err := w.journal.QueryRowContext(ctx, "SELECT coalesce(max(rowid),0) FROM pending_native_inputs").Scan(&limit); err != nil {
		log.Print("native input recovery journal unavailable")
		return
	}
	for last < limit && ctx.Err() == nil {
		var execution, instance int64
		var workspace, session, turn, key string
		var raw, digest, leaseHash []byte
		err := w.journal.QueryRowContext(ctx, `SELECT i.rowid,i.execution_id,i.instance,i.key,i.receipt,i.body_hash,a.workspace,a.lease_hash,p.session_id,t.turn_id
			FROM pending_native_inputs i JOIN harness_processes p ON p.instance=i.instance
			JOIN harness_app_turns t ON t.instance=p.instance JOIN attempts a ON a.lease_hash=p.lease_hash AND a.execution_id=i.execution_id
			WHERE i.rowid>? AND i.rowid<=? AND p.harness='codex' AND p.version=? ORDER BY i.rowid LIMIT 1`, last, limit, w.codexVersion).
			Scan(&last, &execution, &instance, &key, &raw, &digest, &workspace, &leaseHash, &session, &turn)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			log.Print("native input recovery journal unavailable")
			return
		}
		var poll store.TurnInputPoll
		if json.Unmarshal(raw, &poll) != nil || poll.Instance != instance || poll.Session != session || poll.Turn != turn || poll.Key != key {
			continue
		}
		sum := sha256.Sum256([]byte(poll.Lease))
		if !bytes.Equal(sum[:], leaseHash) {
			continue
		}
		call, stop := context.WithTimeout(ctx, 10*time.Second)
		history, err := (codexCLI{bin: w.codex}).readInputHistory(call, workspace, session)
		stop()
		if err != nil || !history.confirms(workspace, poll, digest) {
			continue
		}
		poll.Outcome = "accepted"
		client := turnInputClient{worker: w, execution: execution}
		if err := client.receipt(poll); err != nil {
			log.Printf("execution %d native input history confirmation awaiting recovery", execution)
		}
	}
}
