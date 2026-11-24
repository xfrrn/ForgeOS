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
func (w *worker) probeClientModels(ctx context.Context, core *aioncore.Client, c aioncore.Config, force bool) []store.FeatureModelOption {
	key := c.Instance + "\x00" + c.Backend
	if !force {
		if cached, ok := w.featureModels.Load(key); ok {
			if items, ok := cached.([]store.FeatureModelOption); ok && len(items) > 0 {
				return items
			}
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	db, _, err := w.openRoleJournal(probeCtx, managementChatRole)
	if err != nil {
		return nil
	}
	defer db.Close()
	if _, err = db.ExecContext(probeCtx, `CREATE TABLE IF NOT EXISTS feature_model_probes(instance TEXT,backend TEXT,conversation TEXT NOT NULL,PRIMARY KEY(instance,backend))`); err != nil {
		return nil
	}
	var conv string
	err = db.QueryRowContext(probeCtx, `SELECT conversation FROM feature_model_probes WHERE instance=? AND backend=?`, c.Instance, c.Backend).Scan(&conv)
	createProbe := func() (string, error) {
		root, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir := filepath.Join(root, "ForgeOS", "discussion-workspaces", "model-probe", c.Instance+"-"+c.Backend)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
		id, err := core.Create(probeCtx, c.Backend, dir, "ForgeOS · model probe")
		if err != nil {
			return "", err
		}
		_, _ = db.ExecContext(probeCtx, `INSERT OR REPLACE INTO feature_model_probes(instance,backend,conversation) VALUES(?,?,?)`, c.Instance, c.Backend, id)
		return id, nil
	}
	if errors.Is(err, sql.ErrNoRows) || conv == "" {
		conv, err = createProbe()
		if err != nil {
			return nil
		}
	} else if err != nil {
		return nil
	}
	choices, err := core.ProbeModels(probeCtx, conv)
	if err != nil {
		if errors.Is(err, aioncore.HTTPError(404)) {
			if conv, err = createProbe(); err == nil {
				choices, err = core.ProbeModels(probeCtx, conv)
			}
		}
		if err != nil {
			return nil
		}
	}
	out := make([]store.FeatureModelOption, 0, len(choices)+1)
	seen := make(map[string]bool, len(choices)+1)
	for _, ch := range choices {
		if ch.ID == "" || seen[ch.ID] {
			continue
		}
		seen[ch.ID] = true
		out = append(out, store.FeatureModelOption{ID: ch.ID, Label: ch.Label})
	}
	if c.Model != "" && !seen[c.Model] {
		out = append([]store.FeatureModelOption{{ID: c.Model, Label: c.Model}}, out...)
	}
	if len(out) > 0 {
		w.featureModels.Store(key, out)
	}
	return out
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
		cancel()
		var models []store.FeatureModelOption
		if err == nil && reason == "ready" {
			models = w.probeClientModels(ctx, core, c, gen > 0)
		}
		clients = append(clients, store.FeatureClient{Instance: c.Instance, Backend: c.Backend, Model: c.Model, Status: reason, AvailableModels: models})
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
func discussionWorkspace(job managementChatJob, profile string) (string, error) {
	if job.ProjectPath != "" && filepath.IsAbs(job.ProjectPath) {
		if info, err := os.Stat(job.ProjectPath); err == nil && info.IsDir() {
			return filepath.Clean(job.ProjectPath), nil
		}
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	workspace := filepath.Join(root, "ForgeOS", "discussion-workspaces", profile, fmt.Sprintf("%d-%d", job.ProjectID, job.FeatureID))
	if err := os.MkdirAll(workspace, 0700); err != nil {
		return "", err
	}
	return workspace, nil
}

func (w *worker) resumeFeatureReply(ctx context.Context, db *sql.DB, core *aioncore.Client, job managementChatJob, c aioncore.Config) (string, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS feature_sessions(project INTEGER,feature INTEGER,profile TEXT,conversation TEXT NOT NULL,workspace TEXT NOT NULL DEFAULT '',PRIMARY KEY(project,feature,profile));
 CREATE TABLE IF NOT EXISTS feature_sends(project INTEGER,turn INTEGER,conversation TEXT NOT NULL,marker TEXT NOT NULL,message TEXT NOT NULL DEFAULT '',PRIMARY KEY(project,turn));
 CREATE TABLE IF NOT EXISTS feature_plan_documents(project INTEGER,turn INTEGER,plan TEXT NOT NULL,PRIMARY KEY(project,turn));
 CREATE TABLE IF NOT EXISTS feature_native_receipts(project INTEGER,turn INTEGER,upstream_turn TEXT NOT NULL,PRIMARY KEY(project,turn))`); err != nil {
		return "", err
	}
	// Existing SQLite journals created before workspace tracking need the column added once.
	_, _ = db.ExecContext(ctx, `ALTER TABLE feature_sessions ADD COLUMN workspace TEXT NOT NULL DEFAULT ''`)
	if job.CancelRequested {
		return "", w.cancelSavedFeatureTurn(ctx, db, core, job)
	}
	profile := job.Native.ID.String()
	if job.ProposalID != 0 {
		profile = fmt.Sprintf("%s-proposal-%d", profile, job.ID)
	} else if job.AssistKind != "" {
		profile = fmt.Sprintf("%s-assist-%d", profile, job.ID)
	} else if job.Generation > 0 {
		profile = fmt.Sprintf("%s-%d", profile, job.Generation)
	}
	workspace, err := discussionWorkspace(job, profile)
	if err != nil {
		return "", err
	}
	var conversation, savedWorkspace, marker, message string
	sentErr := db.QueryRowContext(ctx, "SELECT conversation,marker,message FROM feature_sends WHERE project=? AND turn=?", job.ProjectID, job.ID).Scan(&conversation, &marker, &message)
	if sentErr != nil && !errors.Is(sentErr, sql.ErrNoRows) {
		return "", sentErr
	}
	if errors.Is(sentErr, sql.ErrNoRows) {
		lookup := "SELECT conversation,workspace FROM feature_sessions WHERE project=? AND feature=? AND profile=?"
		err = db.QueryRowContext(ctx, lookup, job.ProjectID, job.FeatureID, profile).Scan(&conversation, &savedWorkspace)
		needNewSession := false
		if errors.Is(err, sql.ErrNoRows) {
			if job.ExpectSession {
				return "", aioncore.ErrSessionMissing
			}
			needNewSession = true
		} else if err != nil {
			return "", err
		} else if job.ProjectPath != "" && savedWorkspace != workspace {
			// Upgrade legacy empty-directory discussion sessions to the real project repository.
			needNewSession = true
		}
		if needNewSession {
			id, err := core.Create(ctx, c.Backend, workspace, fmt.Sprintf("ForgeOS · %d/%d", job.ProjectID, job.FeatureID))
			if err != nil {
				return "", err
			}
			if savedWorkspace != workspace && conversation != "" {
				if _, err = db.ExecContext(ctx, "INSERT OR REPLACE INTO feature_sessions(project,feature,profile,conversation,workspace) VALUES(?,?,?,?,?)", job.ProjectID, job.FeatureID, profile, id, workspace); err != nil {
					return "", err
				}
			} else {
				if _, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO feature_sessions(project,feature,profile,conversation,workspace) VALUES(?,?,?,?,?)", job.ProjectID, job.FeatureID, profile, id, workspace); err != nil {
					return "", err
				}
			}
			err = db.QueryRowContext(ctx, lookup, job.ProjectID, job.FeatureID, profile).Scan(&conversation, &savedWorkspace)
			if err != nil {
				return "", err
			}
		}
	}
	if err = core.CheckSession(ctx, conversation); err != nil {
		return "", err
	}
	if errors.Is(sentErr, sql.ErrNoRows) {
		targetModel := c.Model
		if job.RequestedModel != "" {
			targetModel = job.RequestedModel
		}
		if err = core.Configure(ctx, conversation, c.Backend, targetModel); err != nil {
			return "", err
		}
		var files []aioncore.ChatFileRef
		for _, att := range job.Attachments {
			data, dlErr := w.downloadChatAttachment(ctx, att.ID)
			if dlErr != nil {
				return "", dlErr
			}
			uploaded, upErr := core.UploadFile(ctx, conversation, att.Name, data)
			if upErr != nil {
				return "", upErr
			}
			files = append(files, aioncore.ChatFileRef{Kind: "upload", Path: uploaded})
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
		r, sendErr := core.SendWithFiles(ctx, conversation, marker+"\n"+job.Prompt, files)
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
	}
	if (job.ProposalID != 0 || job.AssistKind != "") && c.Backend == "claude" {
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
