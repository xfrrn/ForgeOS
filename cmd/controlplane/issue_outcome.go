package main

import (
	"context"
	"fmt"

	"forgeos/internal/store"
)

func (p deliveryPolicy) reportIssueOutcome(ctx context.Context, data *store.Store, root string) (bool, error) {
	v, err := data.ClaimIssueOutcome(ctx, p.projectID)
	if err != nil || v == nil {
		return false, err
	}
	task, err := data.Task(ctx, v.Task)
	if err != nil {
		return true, err
	}
	project, err := data.Project(ctx, p.projectID)
	if err != nil {
		return true, err
	}
	if task.Spec == nil || task.ProjectID != project.ID || task.Spec.Issue.RepositoryID != project.RepositoryID {
		return true, errIdentity
	}
	repo, err := githubSource(ctx, root, project, ghRead)
	if err != nil {
		return true, err
	}
	path, _, err := checkedFeedbackIssue(ctx, repo, task.Spec.Issue, ghRead)
	if err != nil {
		return true, err
	}
	subject := fmt.Sprintf("ForgeOS task %d", task.ID)
	if v.Execution > 0 {
		subject += fmt.Sprintf(", execution %d", v.Execution)
	}
	var reason string
	switch v.Kind {
	case "execution_abandoned":
		reason = "The execution lease expired and the Owner ended the attempt after reconciliation."
	case "owner_cancelled":
		reason = "The Owner requested cancellation."
	case "source_issue_cancelled":
		reason = "The source Issue was closed."
	case "execution_failed":
		reason = "The Worker reported an execution failure."
		if v.ValidationFailed {
			reason = "Local validation failed."
		}
		if v.ExitCode != nil {
			reason += fmt.Sprintf(" Exit code: %d.", *v.ExitCode)
		}
	default:
		return true, store.ErrConflict
	}
	message := fmt.Sprintf("%s (event %d). %s See the task history for details.", subject, v.Event, reason)
	if err = syncIssueFeedback(ctx, deliveryRequest, path, false, message); err != nil {
		return true, err
	}
	return true, data.ConfirmIssueOutcome(ctx, *v)
}
