package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"forgeos/internal/store"
)

type turnInputClient struct {
	worker    *worker
	execution int64
	lease     string
}

func turnInputPath(execution int64) string {
	return fmt.Sprintf("/internal/executions/%d/turn-inputs", execution)
}

func (c *turnInputClient) run(ctx context.Context, server *codexServer, poll store.TurnInputPoll) {
	for ctx.Err() == nil {
		wait(ctx)
		if poll.Claim == "" {
			var nonce [16]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				return
			}
			poll.Claim = hex.EncodeToString(nonce[:])
		}
		call, cancel := context.WithTimeout(ctx, 4*time.Second)
		var item store.OwnerInstruction
		status, err := c.worker.post(call, turnInputPath(c.execution), poll, &item)
		cancel()
		if err != nil || status != http.StatusOK || !item.Valid() {
			continue
		}
		poll.Claim = ""
		server.mu.Lock()
		if ctx.Err() != nil || server.interrupt || server.turn != poll.Turn {
			server.mu.Unlock()
			return
		}
		pending := poll
		pending.Key = item.Key
		raw, err := json.Marshal(pending)
		if err == nil {
			digest := sha256.Sum256([]byte(item.Body))
			_, err = c.worker.journal.ExecContext(ctx, `INSERT INTO pending_native_inputs(execution_id,key,instance,receipt,body_hash)
				VALUES(?,?,?,?,?)`, c.execution, item.Key, poll.Instance, raw, digest[:])
		}
		if err != nil {
			server.mu.Unlock()
			return
		}
		server.steering[item.Key] = true
		err = server.input.Encode(map[string]any{"id": "forgeos-steer-" + item.Key, "method": "turn/steer", "params": map[string]any{
			"threadId": poll.Session, "expectedTurnId": poll.Turn, "clientUserMessageId": item.Key,
			"input": []map[string]string{{"type": "text", "text": item.Body}},
		}})
		server.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (c *turnInputClient) receipt(poll store.TurnInputPoll) error {
	body, err := json.Marshal(poll)
	if err == nil {
		_, err = c.worker.journal.Exec(`INSERT INTO turn_input_receipts(execution_id,key,receipt) VALUES(?,?,?) ON CONFLICT DO NOTHING`,
			c.execution, poll.Key, body)
	}
	return err
}

func (w *worker) publishTurnInputReceipts(ctx context.Context) {
	var after int64
	for ctx.Err() == nil {
		var row, execution int64
		var key string
		var body json.RawMessage
		err := w.journal.QueryRowContext(ctx, `SELECT rowid,execution_id,key,receipt FROM turn_input_receipts WHERE rowid>? ORDER BY rowid LIMIT 1`, after).Scan(&row, &execution, &key, &body)
		if err == nil {
			after = row
			call, cancel := context.WithTimeout(ctx, 5*time.Second)
			status, postErr := w.post(call, turnInputPath(execution), body, nil)
			cancel()
			if postErr == nil && status == http.StatusNoContent {
				_, err = w.journal.ExecContext(ctx, `DELETE FROM turn_input_receipts WHERE execution_id=? AND key=?`, execution, key)
				if err == nil {
					continue
				}
			} else if ctx.Err() == nil {
				log.Printf("execution %d native input receipt awaiting confirmation (HTTP %d)", execution, status)
			}
		}
		if errors.Is(err, sql.ErrNoRows) {
			after = 0
		} else if err != nil && ctx.Err() == nil {
			log.Print("native input receipt journal unavailable")
		}
		wait(ctx)
	}
}
