package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

const (
	agentLabel = "forgeos-agent"
	pageSize   = 30
)

type issuePoller struct {
	projectID  int64
	validation string
	hookSecret []byte
	wake       chan struct{}
	status     atomic.Pointer[githubPollStatus]
}

type githubPollStatus struct {
	State       string     `json:"state"`
	CheckedAt   time.Time  `json:"checked_at"`
	LastSuccess *time.Time `json:"last_success,omitempty"`
}

type pollStore interface {
	issueStore
	ObserveIssue(context.Context, int64, store.Issue) (int64, error)
	RecordComments(context.Context, int64, store.Issue, []json.RawMessage) error
	ReplaceInbox(context.Context, int64, int64, []store.Discussion) error
	TrackedIssues(context.Context, int64, int64) ([]store.TrackedIssue, error)
	CloseSourceIssue(context.Context, int64, int64, store.TrackedIssue, string, string, time.Time, time.Time) error
}

func pollConfig(id, validation string) (*issuePoller, error) {
	if id == "" && validation == "" {
		return nil, nil
	}
	projectID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || projectID <= 0 || !store.ValidValidation(validation) {
		return nil, errors.New("set a positive FORGEOS_POLL_PROJECT_ID and select auto, docs, go-test or V1 through V4")
	}
	return &issuePoller{projectID: projectID, validation: validation}, nil
}

func configuredPollers(id, validation, file string) (map[int64]*issuePoller, error) {
	legacy, err := pollConfig(id, validation)
	if err != nil {
		return nil, err
	}
	pollers := make(map[int64]*issuePoller)
	if legacy != nil {
		pollers[legacy.projectID] = legacy
	}
	type entry struct {
		Project           int64  `json:"project_id"`
		Validation        string `json:"validation"`
		WebhookSecretFile string `json:"webhook_secret_file"`
	}
	entries, err := readProjectList[entry](file)
	if err != nil {
		return nil, err
	}
	for _, row := range entries {
		p, err := pollConfig(strconv.FormatInt(row.Project, 10), row.Validation)
		if err != nil {
			return nil, err
		}
		if previous := pollers[p.projectID]; previous != nil {
			if previous.validation != p.validation {
				return nil, fmt.Errorf("conflicting poll validation profiles for project %d", p.projectID)
			}
			p = previous
		}
		if row.WebhookSecretFile != "" {
			raw, err := os.ReadFile(row.WebhookSecretFile)
			if err != nil {
				return nil, fmt.Errorf("read webhook secret file for project %d: %w", p.projectID, err)
			}
			secret := bytes.TrimSuffix(bytes.TrimSuffix(raw, []byte("\n")), []byte("\r"))
			if len(bytes.TrimSpace(secret)) == 0 || (p.hookSecret != nil && !bytes.Equal(p.hookSecret, secret)) {
				return nil, fmt.Errorf("empty or conflicting webhook secret for project %d", p.projectID)
			}
			p.hookSecret = secret
			p.wake = make(chan struct{}, 1)
		}
		pollers[p.projectID] = p
	}
	return pollers, nil
}

func (p *issuePoller) start(ctx context.Context, data pollStore, root string, get githubGET) func() {
	ctx, cancel := context.WithCancel(ctx)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for ctx.Err() == nil {
			err := p.scan(ctx, data, root, get)
			if ctx.Err() == nil {
				p.recordStatus(err)
			}
			pause := time.Minute
			if errors.Is(err, errGHLimit) {
				pause = time.Hour
			}
			if err != nil && ctx.Err() == nil {
				log.Printf("GitHub poll for project %d failed, retry in %s: %v", p.projectID, pause, err)
			}
			wake := p.wake
			if err != nil {
				wake = nil // Incoming hints must respect GitHub's retry interval.
			}
			select {
			case <-ctx.Done():
				return
			case <-wake:
			case <-time.After(pause):
			}
		}
	}()
	return func() {
		cancel()
		<-stopped
	}
}

func (p *issuePoller) recordStatus(err error) {
	at := time.Now().UTC()
	current := githubPollStatus{State: "ok", CheckedAt: at, LastSuccess: &at}
	if err != nil {
		current.State, current.LastSuccess = "failed", nil
		if previous := p.status.Load(); previous != nil {
			current.LastSuccess = previous.LastSuccess
		}
		if errors.Is(err, errGHLimit) {
			current.State = "rate_limited"
		}
	}
	p.status.Store(&current)
}

func (p *issuePoller) scan(ctx context.Context, data pollStore, root string, get githubGET) error {
	project, err := data.Project(ctx, p.projectID)
	if err != nil {
		return err
	}
	repo, err := githubSource(ctx, root, project, get)
	if err != nil {
		return err
	}
	var previousFirst, previousLast int64
	repeatedPage := false
	var discussions []store.Discussion
	open := make(map[int64]bool)
	for page := 1; ctx.Err() == nil; page++ {
		path := fmt.Sprintf("repositories/%d/issues?state=open&sort=created&direction=asc&per_page=%d&page=%d", repo.ID, pageSize, page)
		var issues []githubIssue
		if err := get(ctx, path, &issues); err != nil {
			return err
		}
		if len(issues) > pageSize {
			return errors.New("GitHub returned more issues than requested")
		}
		if len(issues) == pageSize {
			first, last := issues[0].ID, issues[len(issues)-1].ID
			if page > 1 && first == previousFirst && last == previousLast {
				if repeatedPage {
					return errors.New("GitHub pagination did not advance")
				}
				repeatedPage = true
				continue
			}
			repeatedPage = false
			previousFirst, previousLast = first, last
		}
		for _, raw := range issues {
			if err := ctx.Err(); err != nil {
				return err
			}
			if raw.State != "open" || raw.PullRequest != nil {
				continue
			}
			issue, err := repo.issue(raw, raw.Number)
			if err == nil {
				open[raw.ID] = true
			}
			var taskID int64
			if err == nil && (!raw.hasAgentLabel() || issue.WorkType == "clarification") {
				if taskID, err = data.ObserveIssue(ctx, p.projectID, issue); err != nil {
					return err
				}
				item := store.Discussion{ID: issue.ID, Number: issue.Number, Title: issue.Title, URL: issue.URL, WorkType: issue.WorkType, SourceDigest: issueSourceDigest(issue)}
				if taskID == 0 && raw.hasAgentLabel() && issue.WorkType == "clarification" {
					item.ClassificationSource = &issue
				}
				discussions = append(discussions, item)
			} else if err == nil {
				var task store.Task
				spec := repo.spec(issue, p.validation, "", validation.Policy{})
				spec.ImportOrigin = "github"
				task, err = data.ImportIssue(ctx, p.projectID, spec)
				taskID = task.ID
			}
			if err == nil && taskID > 0 && raw.Comments > 0 {
				if err = pollComments(ctx, data, get, taskID, issue); errors.Is(err, errGHLimit) {
					return err
				}
			}
			if err != nil {
				log.Printf("GitHub poll project %d issue %d: %v", p.projectID, raw.Number, err)
			}
		}
		if len(issues) < pageSize {
			if err := data.ReplaceInbox(ctx, p.projectID, repo.ID, discussions); err != nil {
				return err
			}
			return p.reconcileClosures(ctx, data, repo, open, get)
		}
	}
	return ctx.Err()
}

func (p *issuePoller) reconcileClosures(ctx context.Context, data pollStore, repo githubProject, open map[int64]bool, get githubGET) error {
	tracked, err := data.TrackedIssues(ctx, p.projectID, repo.ID)
	if err != nil {
		return err
	}
	for _, issue := range tracked {
		if open[issue.ID] {
			continue
		}
		var remote githubIssue
		if err := get(ctx, fmt.Sprintf("repositories/%d/issues/%d", repo.ID, issue.Number), &remote); err != nil {
			if errors.Is(err, errGHLimit) || ctx.Err() != nil {
				return err
			}
			log.Printf("GitHub poll project %d cannot confirm issue %d: %v", p.projectID, issue.Number, err)
			continue
		}
		if remote.State != "closed" || remote.PullRequest != nil {
			continue
		}
		closed, closeErr := time.Parse(time.RFC3339, remote.ClosedAt)
		updated, updateErr := time.Parse(time.RFC3339, remote.UpdatedAt)
		if !repo.matchesIssue(remote, issue.Number) || remote.ID != issue.ID || closeErr != nil || updateErr != nil || updated.Before(closed) {
			log.Printf("GitHub poll project %d rejected inconsistent closure for issue %d", p.projectID, issue.Number)
			continue
		}
		if err := data.CloseSourceIssue(ctx, p.projectID, repo.ID, issue, remote.HTMLURL, remote.StateReason, closed, updated); err != nil {
			return err
		}
	}
	return nil
}

func (i githubIssue) hasAgentLabel() bool {
	for _, label := range i.Labels {
		if strings.EqualFold(label.Name, agentLabel) {
			return true
		}
	}
	return false
}
