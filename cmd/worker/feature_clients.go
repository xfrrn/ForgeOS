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
	ticker := time.NewTicker(40 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		configs, err := featureConfig()
		if err != nil {
			log.Print("feature client configuration unavailable")
		}
		// An unreadable configuration withdraws the earlier report.
		if err = w.reportFeatureClients(ctx, configs); err != nil && ctx.Err() == nil {
			log.Print("feature client status report unavailable")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *worker) reportFeatureClients(ctx context.Context, configs []aioncore.Config) error {
	node := w.id
	if node == "" {
		var err error
		node, err = os.Hostname()
		if err != nil {
			return err
		}
	}
	clients := []store.FeatureClient{}
	for _, c := range configs {
		probe, cancel := context.WithTimeout(ctx, 4*time.Second)
		core, err := aioncore.Login(probe, c)
		if err == nil && core.Available(probe, c.Backend) {
			clients = append(clients, store.FeatureClient{Instance: c.Instance, Backend: c.Backend, Model: c.Model})
		}
		cancel()
	}
	status, err := w.post(ctx, "/internal/management-chat/feature-clients", map[string]any{"node": node, "clients": clients}, nil)
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
			return w.resumeFeatureReply(ctx, db, core, job, c)
		}
	}
	return "", errors.New("native client binding unavailable")
}
func (w *worker) resumeFeatureReply(ctx context.Context, db *sql.DB, core *aioncore.Client, job managementChatJob, c aioncore.Config) (string, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS feature_sessions(project INTEGER,feature INTEGER,profile TEXT,conversation TEXT NOT NULL,PRIMARY KEY(project,feature,profile));
 CREATE TABLE IF NOT EXISTS feature_sends(project INTEGER,turn INTEGER,conversation TEXT NOT NULL,marker TEXT NOT NULL,message TEXT NOT NULL DEFAULT '',PRIMARY KEY(project,turn))`); err != nil {
		return "", err
	}
	profile := job.Native.ID.String()
	var conversation string
	lookup := "SELECT conversation FROM feature_sessions WHERE project=? AND feature=? AND profile=?"
	err := db.QueryRowContext(ctx, lookup, job.ProjectID, job.FeatureID, profile).Scan(&conversation)
	if errors.Is(err, sql.ErrNoRows) {
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
			if _, err = db.ExecContext(ctx, "UPDATE feature_sends SET message=? WHERE project=? AND turn=?", message, job.ProjectID, job.ID); err != nil {
				return "", err
			}
		}
	} else if err != nil {
		return "", err
	}
	return core.Reply(ctx, conversation, message, marker)
}
