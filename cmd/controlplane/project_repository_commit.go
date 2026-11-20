package main

import (
	"context"
	"errors"
	"strings"

	"forgeos/internal/localrepo"
)

type projectCommit struct {
	Branch  string `json:"branch"`
	SHA     string `json:"sha"`
	Subject string `json:"subject,omitempty"`
	Date    string `json:"date,omitempty"`
	Remote  string `json:"remote,omitempty"`
}

func readProjectCommit(ctx context.Context, repo string) (projectCommit, error) {
	out, err := localrepo.Git(ctx, repo, "show", "-s", "--no-show-signature", "--format=%H%x00%s%x00%cI", "HEAD", "--")
	parts := strings.Split(strings.TrimRight(string(out), "\r\n"), "\x00")
	if err != nil || len(parts) != 3 || !gitCommit.MatchString(parts[0]) {
		return projectCommit{}, errors.New("本地仓库没有可读取的 Git 提交。")
	}
	branch, _ := localrepo.Git(ctx, repo, "symbolic-ref", "--quiet", "HEAD")
	return projectCommit{Branch: strings.TrimPrefix(strings.TrimSpace(string(branch)), "refs/heads/"), SHA: parts[0], Subject: parts[1], Date: parts[2]}, nil
}
