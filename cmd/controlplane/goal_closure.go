package main

import (
	"context"
	"fmt"

	"forgeos/internal/store"
)

func (p deliveryPolicy) closeGoal(ctx context.Context, data *store.Store, root string) (bool, error) {
	claim, err := data.ClaimGoalClosure(ctx, p.projectID)
	if err != nil || claim == nil {
		return false, err
	}
	task, err := data.Task(ctx, claim.ID)
	if err != nil {
		return true, err
	}
	project, err := data.Project(ctx, p.projectID)
	if err != nil {
		return true, err
	}
	repo, err := githubSource(ctx, root, project, ghRead)
	if err != nil {
		return true, err
	}
	if task.Spec == nil || task.Plan == nil || task.ProjectID != project.ID || task.Spec.Issue.RepositoryID != repo.ID {
		return true, errIdentity
	}
	issue := task.Spec.Issue
	path := fmt.Sprintf("repositories/%d/issues/%d", repo.ID, issue.Number)
	var remote githubIssue
	if err := ghRead(ctx, path, &remote); err != nil {
		return true, err
	}
	if remote.ID != issue.ID || remote.Number != issue.Number || remote.HTMLURL != issue.URL || remote.RepoURL != "https://api.github.com/repos/"+repo.FullName ||
		remote.PullRequest != nil || (remote.State != "open" && remote.State != "closed") {
		return true, errIdentity
	}
	count := 0
	for _, child := range task.Children {
		if child.Status != "SUPERSEDED" {
			count++
		}
	}
	message := fmt.Sprintf("ForgeOS goal %d completed (plan revision %d). All %d work items have confirmed PR merges and delivery records.", task.ID, task.Plan.Revision, count)
	if err := syncIssueFeedback(ctx, deliveryRequest, path, remote.State == "open", message); err != nil {
		return true, err
	}
	return true, data.CompleteGoal(ctx, *claim)
}
