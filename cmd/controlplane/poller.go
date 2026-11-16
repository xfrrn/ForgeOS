package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/store"
)

const (
	agentLabel = "forgeos-agent"
	pageSize   = 30
)

type issuePoller struct {
	projectID  int64
	validation string
}

type pollStore interface {
	issueStore
	ObserveIssue(context.Context, int64, store.Issue) (int64, error)
	RecordComments(context.Context, int64, store.Issue, []json.RawMessage) error
	ReplaceInbox(context.Context, int64, int64, []store.Discussion) error
}

func pollConfig(id, validation string) (*issuePoller, error) {
	if id == "" && validation == "" {
		return nil, nil
	}
	projectID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || projectID <= 0 || !store.ValidValidation(validation) {
		return nil, errors.New("set a positive FORGEOS_POLL_PROJECT_ID and select docs, go-test or V1 through V4")
	}
	return &issuePoller{projectID, validation}, nil
}

func (p *issuePoller) start(ctx context.Context, data pollStore, root string, get githubGET) func() {
	ctx, cancel := context.WithCancel(ctx)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for ctx.Err() == nil {
			err := p.scan(ctx, data, root, get)
			pause := time.Minute
			if errors.Is(err, errGHLimit) {
				pause = time.Hour
			}
			if err != nil && ctx.Err() == nil {
				log.Printf("GitHub poll for project %d failed, retry in %s: %v", p.projectID, pause, err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(pause):
			}
		}
	}()
	return func() {
		cancel()
		<-stopped
	}
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
			var taskID int64
			if err == nil && !raw.hasAgentLabel() {
				if taskID, err = data.ObserveIssue(ctx, p.projectID, issue); err != nil {
					return err
				}
				discussions = append(discussions, store.Discussion{ID: issue.ID, Number: issue.Number, Title: issue.Title, URL: issue.URL})
			} else if err == nil {
				var task store.Task
				task, err = data.ImportIssue(ctx, p.projectID, store.IssueSpec{Issue: issue, BaseCommit: repo.Base, Validation: p.validation})
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
			return data.ReplaceInbox(ctx, p.projectID, repo.ID, discussions)
		}
	}
	return ctx.Err()
}

func (i githubIssue) hasAgentLabel() bool {
	for _, label := range i.Labels {
		if strings.EqualFold(label.Name, agentLabel) {
			return true
		}
	}
	return false
}
