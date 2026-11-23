package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"forgeos/internal/aioncore"
	"forgeos/internal/store"
	"github.com/google/uuid"
)

func featureConfig() ([]aioncore.Config, error) {
	path := os.Getenv("FORGEOS_FEATURE_CLIENTS_FILE")
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("feature client config requires an absolute path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var conf []aioncore.Config
	if json.Unmarshal(b, &conf) != nil {
		return nil, errors.New("invalid feature client configuration")
	}
	for _, c := range conf {
		if c.Instance == "" || c.Model == "" || (c.Backend != "claude" && c.Backend != "codex") {
			return nil, errors.New("invalid feature client identity")
		}
	}
	return conf, nil
}
func (w *worker) featureClientHeartbeat(ctx context.Context) {
	node := w.id
	if node == "" {
		node, _ = os.Hostname()
	}
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	var next time.Time
	for ctx.Err() == nil {
		var gen int64
		_, err := w.post(ctx, "/internal/management-chat/probe-request", map[string]string{"node": node}, &gen)
		if time.Now().After(next) || (err == nil && gen > 0) {
			configs, configErr := featureConfig()
			if configErr != nil {
				log.Print("feature client configuration unavailable")
			}
			if err := w.reportFeatureClients(ctx, node, configs, gen, configErr != nil); err != nil && ctx.Err() == nil {
				log.Print("feature client status report unavailable")
			}
			next = time.Now().Add(40 * time.Second)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w *worker) reportFeatureClients(ctx context.Context, node string, configs []aioncore.Config, gen int64, configFailed bool) error {
	clients := []store.FeatureClient{}
	for _, c := range configs {
		probe, cancel := context.WithTimeout(ctx, 4*time.Second)
		core, err := aioncore.Login(probe, c)
		reason := aioncore.ProbeFailure(err)
		if err == nil {
			reason = core.Probe(probe, c.Backend)
		}
		clients = append(clients, store.FeatureClient{Instance: c.Instance, Backend: c.Backend, Model: c.Model, Status: reason})
		cancel()
	}
	status, err := w.post(ctx, "/internal/management-chat/feature-clients", map[string]any{"node": node, "clients": clients, "generation": gen, "config_failed": configFailed, "resource_worker": os.Getenv("FORGEOS_FEATURE_RESOURCE_WORKER")}, nil)
	if err == nil && status != 204 {
		err = fmt.Errorf("feature client heartbeat HTTP %d", status)
	}
	return err
}
func (w *worker) nativeFeatureReply(ctx context.Context, job managementChatJob) (string, error) {
	configs, err := featureConfig()
	if err != nil {
		return "", err
	}
	for _, c := range configs {
		if c.Instance == job.Native.Instance && c.Backend == job.Native.Backend && c.Model == job.Native.Model {
			core, err := aioncore.Login(ctx, c)
			if err != nil {
				return "", err
			}
			if !core.Available(ctx, c.Backend) {
				return "", errors.New("native client unavailable")
			}
			db, _, err := w.openRoleJournal(ctx, managementChatRole)
			if err != nil {
				return "", err
			}
			defer db.Close()
			answer, err := w.resumeFeatureReply(ctx, db, core, job, c)
			if !job.CancelRequested && errors.Is(context.Cause(ctx), errOwnerCancel) {
				return "", w.cancelSavedFeatureTurn(ctx, db, core, job)
			}
			return answer, err
		}
	}
	return "", errors.New("native client binding unavailable")
}
func (w *worker) resumeFeatureReply(ctx context.Context, db *sql.DB, core *aioncore.Client, job managementChatJob, c aioncore.Config) (string, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS feature_sessions(project INTEGER,feature INTEGER,profile TEXT,conversation TEXT NOT NULL,PRIMARY KEY(project,feature,profile));
 CREATE TABLE IF NOT EXISTS feature_sends(project INTEGER,turn INTEGER,conversation TEXT NOT NULL,marker TEXT NOT NULL,message TEXT NOT NULL DEFAULT '',PRIMARY KEY(project,turn));
 CREATE TABLE IF NOT EXISTS feature_plan_documents(project INTEGER,turn INTEGER,plan TEXT NOT NULL,PRIMARY KEY(project,turn));
 CREATE TABLE IF NOT EXISTS feature_native_receipts(project INTEGER,turn INTEGER,upstream_turn TEXT NOT NULL,PRIMARY KEY(project,turn))`); err != nil {
		return "", err
	}
	if job.CancelRequested {
		return "", w.cancelSavedFeatureTurn(ctx, db, core, job)
	}
	profile := job.Native.ID.String()
	if job.ProposalID != 0 {
		profile = fmt.Sprintf("%s-proposal-%d", profile, job.ID)
	} else if job.Generation > 0 {
		profile = fmt.Sprintf("%s-%d", profile, job.Generation)
	}
	var conversation string
	lookup := "SELECT conversation FROM feature_sessions WHERE project=? AND feature=? AND profile=?"
	err := db.QueryRowContext(ctx, lookup, job.ProjectID, job.FeatureID, profile).Scan(&conversation)
	if errors.Is(err, sql.ErrNoRows) {
		if job.ExpectSession {
			return "", aioncore.ErrSessionMissing
		}
		root, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		workspace := filepath.Join(root, "ForgeOS", "discussion-workspaces", profile, fmt.Sprintf("%d-%d", job.ProjectID, job.FeatureID))
		if err := os.MkdirAll(workspace, 0700); err != nil {
			return "", err
		}
		id, err := core.Create(ctx, c.Backend, workspace, fmt.Sprintf("ForgeOS · %d/%d", job.ProjectID, job.FeatureID))
		if err != nil {
			return "", err
		}
		if _, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO feature_sessions VALUES(?,?,?,?)", job.ProjectID, job.FeatureID, profile, id); err != nil {
			return "", err
		}
		err = db.QueryRowContext(ctx, lookup, job.ProjectID, job.FeatureID, profile).Scan(&conversation)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if err = core.CheckSession(ctx, conversation); err != nil {
		return "", err
	}
	var marker, message string
	err = db.QueryRowContext(ctx, "SELECT marker,message FROM feature_sends WHERE project=? AND turn=? AND conversation=?", job.ProjectID, job.ID, conversation).Scan(&marker, &message)
	if errors.Is(err, sql.ErrNoRows) {
		if err = core.Configure(ctx, conversation, c.Backend, c.Model); err != nil {
			return "", err
		}
		marker = "[ForgeOS " + uuid.NewString() + "]"
		inserted, err := db.ExecContext(ctx, "INSERT OR IGNORE INTO feature_sends(project,turn,conversation,marker) VALUES(?,?,?,?)", job.ProjectID, job.ID, conversation, marker)
		if err != nil {
			return "", err
		}
		n, err := inserted.RowsAffected()
		if err != nil || n != 1 {
			return "", errors.New("native send already claimed")
		}
		// Commit the local intent before calling upstream. A replay only reads the
		// original conversation, including when the HTTP response was lost.
		r, sendErr := core.Send(ctx, conversation, marker+"\n"+job.Prompt)
		if sendErr == nil {
			message = r.Message
			receiptCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer done()
			if _, err = db.ExecContext(receiptCtx, "INSERT OR REPLACE INTO feature_native_receipts VALUES(?,?,?)", job.ProjectID, job.ID, r.Turn); err != nil {
				return "", err
			}
			if _, err = db.ExecContext(receiptCtx, "UPDATE feature_sends SET message=? WHERE project=? AND turn=?", message, job.ProjectID, job.ID); err != nil {
				return "", err
			}
		}
	} else if err != nil {
		return "", err
	}
	if job.ProposalID != 0 && c.Backend == "claude" {
		var upstream, plan string
		err = db.QueryRowContext(ctx, `SELECT r.upstream_turn,coalesce(d.plan,'') FROM feature_native_receipts r LEFT JOIN feature_plan_documents d USING(project,turn) WHERE r.project=? AND r.turn=?`, job.ProjectID, job.ID).Scan(&upstream, &plan)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		if upstream != "" {
			if plan != "" {
				return plan, core.StopDocumentTurn(ctx, conversation, upstream)
			}
			return core.DocumentReply(ctx, conversation, message, marker, upstream, func(document string) error {
				_, err := db.ExecContext(ctx, "INSERT OR IGNORE INTO feature_plan_documents(project,turn,plan) VALUES(?,?,?)", job.ProjectID, job.ID, document)
				return err
			})
		}
	}
	return core.Reply(ctx, conversation, message, marker)
}
func (w *worker) cancelSavedFeatureTurn(ctx context.Context, db *sql.DB, core *aioncore.Client, job managementChatJob) error {
	stopCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
	defer done()
	var conversation, upstream string
	err := db.QueryRowContext(stopCtx, `SELECT s.conversation,coalesce(r.upstream_turn,'') FROM feature_sends s LEFT JOIN feature_native_receipts r USING(project,turn) WHERE s.project=? AND s.turn=?`, job.ProjectID, job.ID).Scan(&conversation, &upstream)
	if errors.Is(err, sql.ErrNoRows) {
		return errOwnerCancel
	}
	if err != nil || upstream == "" || core.Cancel(stopCtx, conversation, upstream) != nil {
		return errStopUnconfirmed
	}
	return errOwnerCancel
}

var errOwnerCancel = errors.New("owner cancelled this turn")
var errStopUnconfirmed = errors.New("native cancellation unconfirmed")

func (w *worker) watchFeatureTurn(ctx context.Context, stop context.CancelCauseFunc, job managementChatJob) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		var requested bool
		status, err := w.post(ctx, "/internal/management-chat/feature-turn-control", map[string]any{"id": job.ID, "lease": job.Lease}, &requested)
		if err == nil && status == 200 && requested {
			stop(errOwnerCancel)
			return
		}
		if status == 409 {
			stop(errors.New("feature turn lease expired"))
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
