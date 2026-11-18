package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"forgeos/internal/store"
)

var readHarnessProcess = fmt.Sprintf(`SELECT instance,pid,harness,version,coalesce(session_id,''),started_at,exited_at,exit_code,
 coalesce((SELECT previous_session FROM harness_session_handoffs h WHERE h.instance=p.instance AND p.session_id IS NOT NULL),''),
 coalesce((SELECT turn_id FROM harness_app_turns a WHERE a.instance=p.instance),''),
 (SELECT json_group_array(json_object('seq',seq,'call',call,'tool',tool,'phase',phase,'at',at) ORDER BY seq)
  FROM (SELECT * FROM harness_tool_events e WHERE e.instance=p.instance ORDER BY seq DESC LIMIT %d)),
 (SELECT json_object('sequence',sequence,'text',text) FROM harness_model_output o WHERE o.instance=p.instance),
	(SELECT json_object('seq',seq,'call',call,'tool',tool,'text',text,'partial',json(CASE WHEN
	 (SELECT phase FROM harness_tool_events e WHERE e.instance=o.instance AND e.seq=o.seq)='output' THEN 'true' ELSE 'false' END))
	 FROM harness_tool_output o WHERE o.instance=p.instance)
 FROM harness_processes p`, store.ToolWindow)

func scanHarnessProcess(row interface{ Scan(...any) error }) (store.HarnessProcess, error) {
	var p store.HarnessProcess
	var started string
	var exited sql.NullString
	var raw []byte
	var output []byte
	var toolOutput []byte
	err := row.Scan(&p.Instance, &p.PID, &p.Harness, &p.Version, &p.Session, &started, &exited, &p.ExitCode, &p.HandoffFrom, &p.Turn, &raw, &output, &toolOutput)
	if err != nil {
		return p, err
	}
	p.Started, err = time.Parse(time.DateTime, started)
	if err != nil {
		return p, err
	}
	if exited.Valid {
		end, err := time.Parse(time.DateTime, exited.String)
		if err != nil {
			return p, err
		}
		p.Exited = &end
	}
	if len(output) != 0 {
		if err = json.Unmarshal(output, &p.Output); err != nil {
			return p, err
		}
	}
	if len(toolOutput) != 0 {
		if err = json.Unmarshal(toolOutput, &p.ToolOutput); err != nil {
			return p, err
		}
	}
	return p, json.Unmarshal(raw, &p.Tools)
}

type processUpdatesKey struct{}

func (w *worker) reportProcesses(ctx context.Context, job store.Job, updates <-chan struct{}) {
	key := sha256.Sum256([]byte(job.LeaseToken))
	path := fmt.Sprintf("/internal/executions/%d/process", job.ExecutionID)
	timer := time.NewTicker(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-updates:
		case <-timer.C:
		}
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		p, err := scanHarnessProcess(w.journal.QueryRowContext(call, readHarnessProcess+` WHERE lease_hash=? ORDER BY instance DESC LIMIT 1`, key[:]))
		if err == nil {
			status, postErr := w.post(call, path, store.ProcessReport{LeaseToken: job.LeaseToken, Process: p}, nil)
			if ctx.Err() == nil && (postErr != nil || status != 204) {
				log.Printf("execution %d process report not confirmed (HTTP %d)", job.ExecutionID, status)
			}
		} else if ctx.Err() == nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("execution %d process observation unavailable", job.ExecutionID)
		}
		cancel()
	}
}
