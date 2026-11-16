package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"forgeos/internal/store"
)

type worker struct {
	api, token, id, repoRoot, workspaceRoot string
	codex                                   string
	planner                                 bool
	client                                  *http.Client
}

func fromEnv() (*worker, error) {
	if os.Geteuid() == 0 {
		return nil, errors.New("worker requires a non-root user")
	}
	w := &worker{
		api:           os.Getenv("FORGEOS_API_URL"),
		token:         os.Getenv("FORGEOS_WORKER_TOKEN"),
		id:            os.Getenv("FORGEOS_WORKER_ID"),
		repoRoot:      os.Getenv("FORGEOS_REPO_ROOT"),
		workspaceRoot: os.Getenv("FORGEOS_WORKSPACE_ROOT"),
		client:        &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	if w.api == "" {
		w.api = "http://127.0.0.1:8080"
	}
	u, err := url.Parse(w.api)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
		(u.Hostname() != "localhost" && !net.ParseIP(u.Hostname()).IsLoopback()) ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("FORGEOS_API_URL must be a loopback HTTP(S) origin")
	}
	w.api = strings.TrimRight(w.api, "/")
	if w.id == "" {
		w.id, err = os.Hostname()
		if err != nil {
			return nil, err
		}
	}
	if w.token == "" || !filepath.IsAbs(w.repoRoot) || !filepath.IsAbs(w.workspaceRoot) {
		return nil, errors.New("set FORGEOS_WORKER_TOKEN, absolute FORGEOS_REPO_ROOT and FORGEOS_WORKSPACE_ROOT")
	}
	w.repoRoot, err = filepath.EvalSymlinks(w.repoRoot)
	if err != nil {
		return nil, err
	}
	if stat, err := os.Stat(w.repoRoot); err != nil || !stat.IsDir() {
		return nil, errors.New("repository root must be an existing directory")
	}
	if err := os.MkdirAll(w.workspaceRoot, 0700); err != nil {
		return nil, err
	}
	w.workspaceRoot, err = filepath.EvalSymlinks(w.workspaceRoot)
	if err != nil {
		return nil, err
	}
	w.codex, w.planner, err = findCodex(os.Getenv("FORGEOS_CODEX_BIN"))
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (w *worker) post(ctx context.Context, path string, body any, out any) (int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.api+path, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == http.StatusOK {
		const maxJob = 2 << 20
		var data []byte
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxJob+1))
		if err == nil && len(data) > maxJob {
			err = errors.New("claim response too large")
		}
		if err == nil {
			err = json.Unmarshal(data, out)
		}
	}
	return resp.StatusCode, err
}

func wait(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func temporary(status int) bool {
	return status == 0 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func (w *worker) run(ctx context.Context) error {
	claim := map[string]any{"worker": w.id, "workspace_root": w.workspaceRoot}
	if w.codex != "" {
		claim["supports_codex"] = true
		claim["supports_rework"] = true
		if w.planner {
			claim["supports_planner"] = true
		}
	}
	for ctx.Err() == nil {
		var job store.Job
		status, err := w.post(ctx, "/internal/claim", claim, &job)
		if status == http.StatusNoContent || temporary(status) {
			if status != http.StatusNoContent && ctx.Err() == nil {
				log.Printf("claim unavailable (HTTP %d), retrying", status)
			}
			wait(ctx)
			continue
		}
		if err != nil || status != http.StatusOK || job.ExecutionID <= 0 || job.LeaseToken == "" {
			return fmt.Errorf("invalid claim response (HTTP %d)", status)
		}
		result := w.execute(ctx, job)
		report := struct {
			LeaseToken string `json:"lease_token"`
			store.Result
		}{job.LeaseToken, result}
		path := fmt.Sprintf("/internal/executions/%d/result", job.ExecutionID)
		for {
			// Give an interrupted execution one bounded chance to record its outcome on shutdown.
			status, _ = w.post(context.WithoutCancel(ctx), path, report, nil)
			if status == http.StatusNoContent {
				log.Printf("execution %d reported (exit %d)", job.ExecutionID, result.ExitCode)
				break
			}
			if status == http.StatusConflict {
				log.Printf("execution %d result conflicts with the recorded lease or outcome", job.ExecutionID)
				break
			}
			if ctx.Err() != nil || !temporary(status) {
				return fmt.Errorf("execution %d result not acknowledged (HTTP %d)", job.ExecutionID, status)
			}
			log.Printf("execution %d result pending (HTTP %d), retrying", job.ExecutionID, status)
			wait(ctx)
		}
	}
	return nil
}

func main() {
	w, err := fromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := w.run(ctx); err != nil {
		log.Fatal(err)
	}
}
