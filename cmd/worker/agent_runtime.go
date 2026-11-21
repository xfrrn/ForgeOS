package main

import (
	"context"
	"database/sql"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/agents"
	"forgeos/internal/chatkernel"
	"forgeos/internal/store"
)

type agentRuntimeKey struct{}
type agentRuntime struct {
	*agents.Assignment
	worker        *worker
	observedModel string
}

func (w *worker) agentContext(ctx context.Context, a *agents.Assignment) context.Context {
	if a == nil {
		return ctx
	}
	return context.WithValue(ctx, agentRuntimeKey{}, &agentRuntime{Assignment: a, worker: w})
}
func runtimeAgent(ctx context.Context) *agentRuntime {
	a, _ := ctx.Value(agentRuntimeKey{}).(*agentRuntime)
	return a
}
func (a *agentRuntime) requestedModel(harness string) string {
	if a == nil || (a.Harness != "task" && a.Harness != harness) {
		return ""
	}
	return a.Model
}
func (a *agentRuntime) modelArgs(harness string, args []string) []string {
	if model := a.requestedModel(harness); model != "" {
		return append(args, "--model", model)
	}
	return args
}
func (a *agentRuntime) observeModel(model string) {
	if a != nil && model != "" && agents.ValidModel(model) {
		a.observedModel = model
	}
}
func (a *agentRuntime) finish(ctx context.Context, harness, session string, turnErr error) {
	if a == nil {
		return
	}
	call, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Second)
	defer cancel()
	model := a.observedModel
	if harness == "codex" && session != "" && model == "" {
		if cfg, ok := ctx.Value(chatProjectKey{}).(*chatkernel.Config); ok {
			if state, err := os.UserConfigDir(); err == nil {
				model = sessionModelIn(call, session, filepath.Join(state, "ForgeOS", "chat-kernel", strconv.FormatInt(cfg.ProjectID, 10)))
			}
		} else if a.worker.role != managementChatRole {
			model = codexSessionModel(call, session)
		}
	}
	outcome := "completed"
	if turnErr != nil {
		outcome = "failed"
	}
	observation := store.AgentObservation{Ticket: a.Ticket, Harness: harness, Model: model, Session: session, Outcome: outcome}
	for attempt := 0; attempt < 2; attempt++ {
		status, err := a.worker.post(call, "/internal/agent-observations", observation, nil)
		if err == nil && status == 204 {
			return
		}
		if !temporary(status) || call.Err() != nil || attempt == 1 {
			log.Printf("could not record agent observation for %s (HTTP %d)", a.Key, status)
			return
		}
	}
}

// CLI JSON events can omit the model. Read only metadata for the returned session.
func codexSessionModel(ctx context.Context, session string) string {
	if !nativeSessionID.MatchString(session) {
		return ""
	}
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".codex")
	}
	return sessionModelIn(ctx, session, dir)
}

func sessionModelIn(ctx context.Context, session, dir string) string {
	path := filepath.Join(dir, "state_5.sqlite")
	u := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/"), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return ""
	}
	defer db.Close()
	var model string
	if db.QueryRowContext(ctx, "SELECT model FROM threads WHERE id=?", session).Scan(&model) != nil || !agents.ValidModel(model) {
		return ""
	}
	return model
}
