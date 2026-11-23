package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func featureRef(parent, task, execution int64) string {
	return fmt.Sprintf("refs/forgeos/features/%d/%d/%d", parent, task, execution)
}

func pinFeatureStep(ctx context.Context, repo string, job store.Job, sha string) error {
	ref := featureRef(job.Spec.FeatureStep.Parent, job.TaskID, job.ExecutionID)
	if _, err := localrepo.Git(ctx, repo, "update-ref", ref, sha, strings.Repeat("0", len(sha))); err != nil {
		actual, readErr := localrepo.Git(ctx, repo, "rev-parse", "--verify", ref)
		if readErr != nil || strings.TrimSpace(string(actual)) != sha {
			return fmt.Errorf("无法固定实现任务 T-%d 的提交", job.TaskID)
		}
	}
	return nil
}

// Each intermediate commit has the original base as its sole parent. Three-way
// merging the next sibling therefore always compares it against that same base.
func assembleFeature(ctx context.Context, repo string, job store.Job) (string, error) {
	if len(job.FeatureInputs) < 2 {
		return "", errors.New("缺少实现任务的提交清单")
	}
	_, err := localrepo.Git(ctx, repo, "config", "--get-regexp", `^merge\..*\.driver$`)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return "", errors.New("合成需要使用 Git 内置合并驱动")
	}
	if err := checkGitFilters(ctx, repo); err != nil {
		return "", err
	}
	base := job.Spec.BaseCommit
	combined := ""
	for _, step := range job.FeatureInputs {
		ref := featureRef(job.TaskID, step.Task, step.Execution)
		parents, err := localrepo.Git(ctx, repo, "rev-list", "-1", "--parents", ref, "--")
		if err != nil || strings.TrimSpace(string(parents)) != step.Commit+" "+base {
			return "", fmt.Errorf("任务「%s」的提交或共同基线无法核实", step.Name)
		}
		if combined == "" {
			combined = step.Commit
			continue
		}
		raw, err := localrepo.Git(ctx, repo, "merge-tree", "--write-tree", "--name-only", "--no-messages", combined, step.Commit)
		if err != nil {
			if errors.As(err, &exit) && exit.ExitCode() == 1 {
				_, paths, _ := strings.Cut(strings.TrimSpace(string(raw)), "\n")
				return "", fmt.Errorf("合成「%s」发生冲突，请修订相应实现任务。%s", step.Name, strings.Join(strings.Fields(paths), ", "))
			}
			return "", fmt.Errorf("合成「%s」失败: %w", step.Name, err)
		}
		tree := strings.TrimSpace(string(raw))
		combined, err = createCandidateCommit(ctx, repo, tree, base, job.Spec.Issue.Number)
		if err != nil {
			return "", err
		}
	}
	tree, err := localrepo.Git(ctx, repo, "rev-parse", combined+"^{tree}")
	if err != nil {
		return "", err
	}
	if err = validateCandidateTree(ctx, repo, base, base, strings.TrimSpace(string(tree))); err != nil {
		return "", err
	}
	return combined, nil
}
