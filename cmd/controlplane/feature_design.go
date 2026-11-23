package main

import (
	"net/http"

	"forgeos/internal/store"
)

// The browser names the inspected final turn. The server loads the dispatch text from that turn.
func (chat projectChat) dispatchDesign(w http.ResponseWriter, r *http.Request, p, f, round int64) {
	var in store.DesignDispatch
	if !decodeLimited(w, r, &in, 64<<10) {
		return
	}
	if in.Turn <= 0 || in.Revision <= 0 || in.Issue < 0 {
		problem(w, 400, "请选择已拍板方案的设计定稿及有效 Issue 编号")
		return
	}
	ctx := r.Context()
	receipt, err := chat.data.DesignReceipt(ctx, p, f, round)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	var repo githubProject
	var issue *store.Issue
	var digest string
	if receipt.FinalTurn != in.Turn {
		if in.Issue == 0 {
			repo, _, err = chat.issueRepository(ctx, p)
		} else {
			var item store.Issue
			repo, item, err = chat.readIssue(ctx, p, in.Issue)
			issue = &item
			digest = issueSourceDigest(item)
		}
		if err != nil {
			problem(w, 502, "无法读取本项目 GitHub 仓库或开放 Issue，请核对编号、权限和网络后重试")
			return
		}
	}
	action, err := chat.data.PrepareDesignDispatch(ctx, p, f, round, in, repo.FullName, repo.ID, digest, issue)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	chat.confirmAction(w, r, p, action, chatIssueDraft{})
}
