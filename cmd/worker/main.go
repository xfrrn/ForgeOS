package main

import (
	"bytes"
	"context"
	"database/sql"
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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"forgeos/internal/store"
)

type worker struct {
	api, token, id, repoRoot, workspaceRoot string
	codex, codexVersion                     string
	claude                                  claudeCLI
	planner                                 bool
	slots                                   int
	modelSlots, ciSlots                     int
	client                                  *http.Client
	journal                                 *sql.DB
}

func fromEnv(role string) (*worker, error) {
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
	if role == "reviewer" {
		w.token = os.Getenv("FORGEOS_REVIEWER_TOKEN")
		info, err := os.Stat(w.repoRoot)
		if strings.TrimSpace(w.token) == "" || !filepath.IsAbs(w.repoRoot) || err != nil || !info.IsDir() {
			return nil, errors.New("set FORGEOS_REVIEWER_TOKEN and an existing absolute FORGEOS_REPO_ROOT")
		}
		return w, nil
	}
	if role == "management" || role == "test-agent" {
		w.token = os.Getenv("FORGEOS_MANAGEMENT_TOKEN")
		if role == "test-agent" {
			w.token = os.Getenv("FORGEOS_TEST_AGENT_TOKEN")
		}
		if strings.TrimSpace(w.token) == "" {
			return nil, fmt.Errorf("set the dedicated %s token", role)
		}
		w.codex, w.planner, err = findCodex(os.Getenv("FORGEOS_CODEX_BIN"))
		if err != nil {
			return nil, err
		}
		if !w.planner {
			return nil, fmt.Errorf("%s requires local Codex with structured output", role)
		}
		return w, nil
	}
	if w.id == "" {
		w.id, err = os.Hostname()
		if err != nil {
			return nil, err
		}
	}
	if w.slots, err = workerSlots("FORGEOS_WORKER_SLOTS", 1); err != nil {
		return nil, err
	}
	if w.modelSlots, err = workerSlots("FORGEOS_WORKER_MODEL_SLOTS", w.slots); err != nil {
		return nil, err
	}
	if w.ciSlots, err = workerSlots("FORGEOS_WORKER_CI_SLOTS", w.slots); err != nil {
		return nil, err
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
	if w.codex != "" {
		w.codexVersion, err = readCodexVersion(w.codex)
		if err != nil {
			if os.Getenv("FORGEOS_CODEX_BIN") != "" {
				return nil, err
			}
			log.Printf("skipping discovered Codex: %v", err)
			w.codex, w.planner = "", false
		}
	}
	w.claude, err = findClaude(os.Getenv("FORGEOS_CLAUDE_BIN"))
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

func workerSlots(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return int(n), nil
}

func temporary(status int) bool {
	return status == 0 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func (w *worker) run(ctx context.Context) error {
	journal, err := w.openJournal(ctx)
	if err != nil {
		return err
	}
	w.journal = journal
	defer journal.Close()
	writer := w.codex != "" || w.claude.bin != ""
	caps := store.WorkerCapabilities{SupportsResources: true, SupportsValidation: true, SupportsDeclaredChecks: true, SupportsRiskPolicy: true,
		SupportsCodex: w.codex != "", SupportsClaude: w.claude.bin != "", SupportsPlanner: w.planner, SupportsRework: writer, SupportsCheckpoints: writer}
	h := store.WorkerHeartbeat{Worker: w.id, OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(),
		Slots: w.slots, ModelSlots: w.modelSlots, CISlots: w.ciSlots, Capabilities: caps}
	beat := func(ctx context.Context) error {
		for ctx.Err() == nil {
			status, err := w.post(ctx, "/internal/workers/heartbeat", h, nil)
			if err == nil && status == http.StatusNoContent {
				return nil
			}
			if !temporary(status) {
				return fmt.Errorf("Worker heartbeat rejected (HTTP %d)", status)
			}
			wait(ctx)
		}
		return nil
	}
	if err := beat(ctx); err != nil {
		return err
	}
	child, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-child.Done():
				done <- nil
				return
			case <-t.C:
				if err := beat(child); err != nil {
					stop()
					done <- err
					return
				}
			}
		}
	}()
	if err = w.replayReceipts(child); err == nil {
		err = w.runQueue(child, caps)
	}
	stop()
	return errors.Join(err, <-done)
}

func (w *worker) runQueue(parent context.Context, caps store.WorkerCapabilities) error {
	ctx, stop := context.WithCancelCause(parent)
	var jobs sync.WaitGroup
	defer jobs.Wait()
	defer stop(nil)
	occupied := make(chan struct{}, w.slots)
	claim := struct {
		Worker        string `json:"worker"`
		WorkspaceRoot string `json:"workspace_root"`
		store.WorkerCapabilities
	}{w.id, w.workspaceRoot, caps}
	for ctx.Err() == nil {
		select {
		case occupied <- struct{}{}:
		case <-ctx.Done():
			continue
		}
		var job store.Job
		requested := time.Now()
		status, err := w.post(ctx, "/internal/claim", claim, &job)
		if status == http.StatusNoContent || temporary(status) {
			<-occupied
			if status != http.StatusNoContent && ctx.Err() == nil {
				log.Printf("claim unavailable (HTTP %d), retrying", status)
			}
			wait(ctx)
			continue
		}
		if err != nil || status != http.StatusOK || job.ExecutionID <= 0 || job.LeaseToken == "" {
			return fmt.Errorf("invalid claim response (HTTP %d)", status)
		}
		jobs.Add(1)
		go func() {
			defer jobs.Done()
			defer func() { <-occupied }()
			if err := w.executeAndReport(ctx, job, requested); err != nil {
				stop(err)
			}
		}()
	}
	if parent.Err() != nil {
		return nil
	}
	return context.Cause(ctx)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) == 2 && os.Args[1] == "review-test-plan" {
		if err := runTestCoverage(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "plan-tests" {
		if err := runTestPlan(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "validate-candidate" {
		if err := runCICandidate(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "review-candidate" {
		if err := runCandidateReview(ctx, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	var role string
	if len(os.Args) == 2 {
		role = os.Args[1]
	}
	if len(os.Args) > 1 && role != "management" && role != "reviewer" && role != "test-agent" {
		log.Fatal("usage: worker [management|reviewer|test-agent|review-candidate|validate-candidate|plan-tests|review-test-plan]")
	}
	w, err := fromEnv(role)
	if err != nil {
		log.Fatal(err)
	}
	if role == "test-agent" {
		err = w.runTestAgent(ctx)
	} else if role == "reviewer" {
		err = w.runReviewer(ctx)
	} else if role == "management" {
		err = w.runManagement(ctx)
	} else {
		err = w.run(ctx)
	}
	if err != nil {
		log.Fatal(err)
	}
}
