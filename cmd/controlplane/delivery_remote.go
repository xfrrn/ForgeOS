package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

type deliveryAPI func(ctx context.Context, method, endpoint string, body, dest any) error
type deliveryPush func(ctx context.Context, repo, slug, branch, sha string) error

func advanceGitHub(ctx context.Context, repo string, task store.Task, d store.Delivery) (store.Delivery, error) {
	return advanceRemote(ctx, repo, task, d, deliveryRequest, publishCandidate)
}

func deliveryRequest(ctx context.Context, method, endpoint string, body, dest any) error {
	if method == "GET" {
		return ghRead(ctx, endpoint, dest)
	}
	input, err := json.Marshal(body)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "gh", "api", "--hostname", "github.com", "--method", method,
		"-H", "X-GitHub-Api-Version: 2026-03-10", "--input", "-", endpoint)
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return deliveryRequestError(err)
	}
	if dest != nil {
		if err := json.Unmarshal(output, dest); err != nil {
			return errors.New("unexpected GitHub response; will reconcile remote state on retry")
		}
	}
	return nil
}

var ghHTTPStatus = regexp.MustCompile(`(?m)\(HTTP ([45][0-9]{2})\)\r?$`)

func deliveryRequestError(err error) error {
	// Only keep gh's numeric HTTP status. Its diagnostic can contain remote text or credential details.
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status := ghHTTPStatus.FindSubmatch(exit.Stderr); status != nil {
			hint := "will reconcile remote state on retry"
			switch string(status[1]) {
			case "401", "403", "404":
				hint = "check repository access, credentials and repository rules"
			case "405", "409", "422":
				hint = "check remote state and repository rules"
			case "429":
				hint = "request rate limited; will retry"
			}
			return fmt.Errorf("GitHub HTTP %s: %s", status[1], hint)
		}
	}
	return errors.New("GitHub operation unconfirmed; will reconcile remote state on retry")
}

type remotePull struct {
	Number     int64  `json:"number"`
	HTMLURL    string `json:"html_url"`
	State      string `json:"state"`
	Merged     bool   `json:"merged"`
	MergeSHA   string `json:"merge_commit_sha"`
	Head, Base struct {
		Ref, SHA string
		Repo     struct{ ID int64 }
	}
}

func (p remotePull) matches(i store.Issue, d store.Delivery) bool {
	return p.Number > 0 && (d.PRNumber == 0 || p.Number == d.PRNumber) &&
		p.HTMLURL == fmt.Sprintf("https://github.com/%s/pull/%d", i.Repository, p.Number) &&
		p.Base.Repo.ID == i.RepositoryID && p.Head.Repo.ID == i.RepositoryID &&
		p.Base.Ref == d.TargetBranch && p.Head.Ref == d.Branch && p.Head.SHA == d.Commit
}

func waitForRemote(d store.Delivery, reason string) (store.Delivery, error) {
	if d.MergeSHA == "" {
		d.State = "WAITING"
	}
	d.Detail = reason
	return d, nil
}

func advanceRemote(ctx context.Context, repo string, task store.Task, d store.Delivery, api deliveryAPI, push deliveryPush) (store.Delivery, error) {
	if task.Spec == nil || task.Kind != "codex_issue" || d.TaskID != task.ID || !gitCommit.MatchString(d.Commit) {
		return d, errIdentity
	}
	i := task.Spec.Issue
	origin, err := localrepo.Git(ctx, repo, "config", "--get", "remote.origin.url")
	if err != nil || !strings.EqualFold(repositoryName(strings.TrimSpace(string(origin))), i.Repository) {
		return waitForRemote(d, "registered origin no longer matches the approved repository")
	}
	path := fmt.Sprintf("repositories/%d", i.RepositoryID)
	var metadata struct {
		ID            int64
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := api(ctx, "GET", path, nil, &metadata); err != nil {
		return d, err
	}
	if metadata.ID != i.RepositoryID || !strings.EqualFold(metadata.FullName, i.Repository) {
		return waitForRemote(d, "GitHub repository identity changed")
	}
	issuePath := fmt.Sprintf("%s/issues/%d", path, i.Number)
	var issue githubIssue
	if err := api(ctx, "GET", issuePath, nil, &issue); err != nil {
		return d, err
	}
	if issue.ID != i.ID || issue.Number != i.Number || issue.HTMLURL != i.URL || issue.PullRequest != nil ||
		issue.RepoURL != "https://api.github.com/repos/"+metadata.FullName || (issue.State != "open" && issue.State != "closed") {
		return waitForRemote(d, "GitHub issue identity changed")
	}
	target := d.TargetBranch
	if target == "" {
		target = metadata.DefaultBranch
	}
	for _, name := range []string{target, d.Branch} {
		if _, err := localrepo.Git(ctx, repo, "check-ref-format", "refs/heads/"+name); err != nil || name == "" || name == "HEAD" {
			return waitForRemote(d, "invalid delivery branch")
		}
	}
	if d.Branch == target {
		return waitForRemote(d, "delivery branch collides with the target")
	}
	if d.TargetBranch == "" {
		d.TargetBranch, d.Detail = target, "resolved target branch"
		return d, nil
	}

	pulls := path + "/pulls"
	if d.PRNumber == 0 {
		var found []remotePull
		owner, _, _ := strings.Cut(i.Repository, "/")
		query := url.Values{"state": {"all"}, "head": {owner + ":" + d.Branch}, "per_page": {"2"}}
		if err := api(ctx, "GET", pulls+"?"+query.Encode(), nil, &found); err != nil {
			return d, err
		}
		var pr remotePull
		switch len(found) {
		case 0:
			if issue.State != "open" {
				return waitForRemote(d, "issue closed before delivery")
			}
			if err := confirmBase(ctx, api, path, repo, d, task.Spec.BaseCommit); err != nil {
				return d, err
			}
			if err := push(ctx, repo, i.Repository, d.Branch, d.Commit); err != nil {
				return d, err
			}
			body := map[string]any{
				"title": fmt.Sprintf("Deliver issue #%d", i.Number), "head": d.Branch, "base": target,
				"body": fmt.Sprintf("Refs #%d\n\nForgeOS task %d, execution %d.\n\nValidated and approved commit: `%s` (%s).", i.Number, task.ID, d.ExecutionID, d.Commit, task.Spec.Validation),
			}
			if task.GoalID != 0 {
				body["body"] = fmt.Sprintf("%s\n\nGoal %d, plan revision %d, work item %s.", body["body"], task.GoalID, task.PlanRevision, task.PlanKey)
			}
			if err := api(ctx, "POST", pulls, body, &pr); err != nil {
				return d, err
			}
		case 1:
			pr = found[0]
		default:
			return waitForRemote(d, "multiple pull requests use the delivery branch")
		}
		if !pr.matches(i, d) {
			return waitForRemote(d, "pull request source, target or candidate changed")
		}
		d.PRNumber, d.PRURL, d.State, d.Detail = pr.Number, pr.HTMLURL, "WAITING", "pull request recorded"
		return d, nil
	}

	prPath := fmt.Sprintf("%s/%d", pulls, d.PRNumber)
	var pr remotePull
	if err := api(ctx, "GET", prPath, nil, &pr); err != nil {
		return d, err
	}
	if !pr.matches(i, d) {
		return waitForRemote(d, "pull request source, target or candidate changed")
	}
	if pr.Merged {
		if !gitCommit.MatchString(pr.MergeSHA) || (d.MergeSHA != "" && d.MergeSHA != pr.MergeSHA) {
			return waitForRemote(d, "merge commit could not be verified")
		}
		if d.MergeSHA == "" {
			// A human may have merged this PR while the reconciler was offline. A regular merge has two parents.
			var merged struct {
				SHA     string
				Parents []struct{ SHA string }
			}
			if err := api(ctx, "GET", path+"/git/commits/"+pr.MergeSHA, nil, &merged); err != nil {
				return d, err
			}
			if merged.SHA != pr.MergeSHA || len(merged.Parents) != 1 || !gitCommit.MatchString(merged.Parents[0].SHA) {
				return waitForRemote(d, "merged pull request does not have a verified single-parent result")
			}
			d.MergeSHA, d.State, d.Detail = pr.MergeSHA, "MERGED", "merge commit recovered from GitHub"
			return d, nil
		}
		if err := finishIssue(ctx, api, issuePath, issue.State == "open" && task.GoalID == 0, d); err != nil {
			return d, err
		}
		d.State, d.Detail = "DONE", "merge and issue feedback confirmed"
		return d, nil
	}
	if d.MergeSHA != "" || pr.State != "open" || issue.State != "open" {
		return waitForRemote(d, "pull request or issue state requires reconciliation")
	}
	if err := confirmBase(ctx, api, path, repo, d, task.Spec.BaseCommit); err != nil {
		return d, err
	}
	reason, err := mergeGates(ctx, api, i, d)
	if err != nil {
		return d, err
	}
	if reason != "" {
		return waitForRemote(d, reason)
	}
	// GitHub rechecks protections (even for administrators) and the pinned head when executing the merge.
	request := map[string]any{"merge_method": "squash", "sha": d.Commit, "bypass_rules": false, "merge_action": "direct_merge"}
	if err := api(ctx, "PUT", prPath+"/merge-async", request, nil); err != nil {
		return d, err
	}
	return waitForRemote(d, "squash merge requested; awaiting confirmed merge SHA")
}

func confirmBase(ctx context.Context, api deliveryAPI, path, repo string, d store.Delivery, base string) error {
	data, err := localrepo.Git(ctx, repo, "cat-file", "-p", d.Commit)
	header, _, _ := strings.Cut(string(data), "\n\n")
	var parents []string
	for _, line := range strings.Split(header, "\n") {
		if sha, ok := strings.CutPrefix(line, "parent "); ok {
			parents = append(parents, sha)
		}
	}
	if err != nil || !gitCommit.MatchString(base) || len(parents) != 1 || parents[0] != base {
		return errors.New("local candidate is missing or does not have the validated base as its only parent")
	}
	target, err := deliveryTarget(ctx, api, path, d.TargetBranch)
	if err != nil {
		return err
	}
	if target == base {
		return nil
	}
	var comparison struct {
		MergeBase struct{ SHA string } `json:"merge_base_commit"`
	}
	if err := api(ctx, "GET", path+"/compare/"+base+"..."+target+"?per_page=1", nil, &comparison); err != nil {
		return err
	}
	if comparison.MergeBase.SHA != base {
		return errors.New("validated base is not in the target branch history")
	}
	return nil
}

func deliveryTarget(ctx context.Context, api deliveryAPI, path, branch string) (string, error) {
	var ref struct {
		Ref    string
		Object struct{ Type, SHA string }
	}
	if err := api(ctx, "GET", path+"/git/ref/heads/"+url.PathEscape(branch), nil, &ref); err != nil {
		return "", err
	}
	if ref.Ref != "refs/heads/"+branch || ref.Object.Type != "commit" || !gitCommit.MatchString(ref.Object.SHA) {
		return "", errIdentity
	}
	return ref.Object.SHA, nil
}

func mergeGates(ctx context.Context, api deliveryAPI, i store.Issue, d store.Delivery) (string, error) {
	const query = `query($owner:String!,$repo:String!,$pr:Int!){
	 repository(owner:$owner,name:$repo){databaseId pullRequest(number:$pr){
	  headRefOid baseRefName headRefName isDraft mergeable mergeStateStatus reviewDecision
	  commits(last:1){nodes{commit{oid statusCheckRollup{state}}}}
	 }}}`
	var response struct {
		Data struct {
			Repository struct {
				DatabaseID  int64
				PullRequest struct {
					HeadRefOid, BaseRefName, HeadRefName, Mergeable, MergeStateStatus string
					ReviewDecision                                                    json.RawMessage
					IsDraft                                                           *bool
					Commits                                                           struct {
						Nodes []struct {
							Commit struct {
								Oid               string
								StatusCheckRollup json.RawMessage
							}
						}
					}
				}
			}
		}
		Errors []json.RawMessage
	}
	owner, repo, _ := strings.Cut(i.Repository, "/")
	request := map[string]any{"query": query, "variables": map[string]any{"owner": owner, "repo": repo, "pr": d.PRNumber}}
	if err := api(ctx, "POST", "graphql", request, &response); err != nil {
		return "", err
	}
	p := response.Data.Repository.PullRequest
	if len(response.Errors) != 0 || response.Data.Repository.DatabaseID != i.RepositoryID || p.HeadRefOid != d.Commit ||
		p.BaseRefName != d.TargetBranch || p.HeadRefName != d.Branch || len(p.Commits.Nodes) != 1 || p.Commits.Nodes[0].Commit.Oid != d.Commit {
		return "GitHub has not confirmed merge gates for the accepted head", nil
	}
	var rollup *struct{ State string }
	if json.Unmarshal(p.Commits.Nodes[0].Commit.StatusCheckRollup, &rollup) != nil {
		return "GitHub has not confirmed check status for the accepted head", nil
	}
	if rollup != nil && rollup.State != "SUCCESS" {
		if rollup.State == "FAILURE" || rollup.State == "ERROR" {
			return "GitHub checks failed for the accepted head", nil
		}
		return "waiting for successful GitHub checks", nil
	}
	if p.MergeStateStatus == "BEHIND" {
		return "accepted candidate is behind the protected target branch", nil
	}
	var review *string
	if p.IsDraft == nil || *p.IsDraft || p.Mergeable != "MERGEABLE" || p.MergeStateStatus != "CLEAN" ||
		json.Unmarshal(p.ReviewDecision, &review) != nil || (review != nil && *review != "APPROVED") {
		return "waiting for GitHub reviews, branch protection or conflict resolution", nil
	}
	return "", nil
}

func finishIssue(ctx context.Context, api deliveryAPI, path string, closeIssue bool, d store.Delivery) error {
	message := fmt.Sprintf("ForgeOS task %d delivered in %s.\n\nAccepted commit: `%s`\nMerge commit: `%s`", d.TaskID, d.PRURL, d.Commit, d.MergeSHA)
	return syncIssueFeedback(ctx, api, path, closeIssue, message)
}

func syncIssueFeedback(ctx context.Context, api deliveryAPI, path string, closeIssue bool, message string) error {
	found := false
	for page := 1; !found; page++ {
		var comments []struct{ Body string }
		if err := api(ctx, "GET", path+"/comments?per_page=100&page="+strconv.Itoa(page), nil, &comments); err != nil {
			return err
		}
		for _, c := range comments {
			if c.Body == message {
				found = true
				break
			}
		}
		if len(comments) < 100 {
			break
		}
	}
	if !found {
		if err := api(ctx, "POST", path+"/comments", map[string]any{"body": message}, nil); err != nil {
			return err
		}
	}
	if closeIssue {
		var closed githubIssue
		if err := api(ctx, "PATCH", path, map[string]any{"state": "closed", "state_reason": "completed"}, &closed); err != nil {
			return err
		}
		if closed.State != "closed" {
			return errors.New("GitHub has not confirmed issue closure")
		}
	}
	return nil
}

func publishCandidate(ctx context.Context, repo, slug, branch, sha string) error {
	return pushFromIsolatedRepo(ctx, repo, "https://github.com/"+slug+".git", "https", branch, sha)
}

func pushFromIsolatedRepo(ctx context.Context, repo, remote, protocol, branch, sha string) error {
	objects, err := localrepo.Git(ctx, repo, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return errors.New("candidate object database is unavailable")
	}
	// Only object data is shared with the project. Remote rewrites, hooks and credentials stay out of the push.
	tmp, err := os.MkdirTemp("", "forgeos-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	format := "sha1"
	if len(sha) == 64 {
		format = "sha256"
	}
	if _, err := localrepo.Git(ctx, tmp, "init", "--bare", "--template=", "--object-format="+format, "."); err != nil {
		return errors.New("cannot initialize isolated Git transport")
	}
	if err := os.WriteFile(filepath.Join(tmp, "objects", "info", "alternates"), []byte(strings.TrimSpace(string(objects))+"\n"), 0600); err != nil {
		return err
	}
	shallow, err := localrepo.Git(ctx, repo, "rev-parse", "--path-format=absolute", "--git-path", "shallow")
	if err != nil {
		return errors.New("cannot inspect repository depth")
	}
	boundaries, err := os.ReadFile(strings.TrimSpace(string(shallow)))
	if err == nil {
		if err := os.WriteFile(filepath.Join(tmp, "shallow"), boundaries, 0600); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect shallow boundaries")
	}
	opts := []string{"-c", "protocol." + protocol + ".allow=always", "-c", "http.followRedirects=false", "-c", "credential.helper=", "-c", "credential.https://github.com.helper=!gh auth git-credential"}
	ref := "refs/heads/" + branch
	current, err := localrepo.Git(ctx, tmp, append(opts, "ls-remote", "--refs", remote, ref)...)
	if err != nil {
		return errors.New("could not inspect remote delivery branch")
	}
	if len(current) > 0 {
		if strings.TrimSpace(string(current)) != sha+"\t"+ref {
			return errors.New("delivery branch already points to a different commit")
		}
		return nil
	}
	// The empty expected value makes branch creation compare-and-swap, including concurrent writers.
	if _, err := localrepo.Git(ctx, tmp, append(opts, "push", "--no-follow-tags", "--force-with-lease="+ref+":", remote, sha+":"+ref)...); err != nil {
		return errors.New("branch creation unconfirmed; will inspect the remote again")
	}
	return nil
}
