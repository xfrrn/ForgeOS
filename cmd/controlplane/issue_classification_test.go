package main

import (
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func TestIssueWorkType(t *testing.T) {
	for _, tt := range []struct {
		title  string
		labels []string
		want   string
	}{
		{"Add release notes", []string{agentLabel}, "development"},
		{"[Code Review] Candidate", nil, "code-review"},
		{"[Testing] Candidate", []string{"FORGEOS:TYPE:TESTING"}, "testing"},
		{"[Investigation] Lock", []string{"forgeos:type:testing"}, "clarification"},
		{"[Testing] Lock", []string{"forgeos:type:investigation", "forgeos:type:testing"}, "clarification"},
		{"Restart service", []string{agentLabel}, "operations"},
		{"Investigate a delay", []string{agentLabel}, "investigation"},
		{"Implement a fix", []string{agentLabel, "forgeos:type:unknown"}, "clarification"},
		{"Discuss the release", []string{agentLabel}, "clarification"},
		{"[调查] [运维] 确认发布范围", nil, "clarification"},
		{"Test the release and deploy production", []string{agentLabel}, "clarification"},
		{"Investigate a delay", nil, ""},
	} {
		remote := githubIssue{ID: 1, Number: 1, Title: tt.title, RepoURL: "https://api.github.com/repos/a/b", HTMLURL: "https://github.com/a/b/issues/1", State: "open", UpdatedAt: "2026-01-01T00:00:00Z"}
		for _, l := range tt.labels {
			remote.Labels = append(remote.Labels, struct{ Name string }{l})
		}
		repo := githubProject{FullName: "a/b", Harness: "claude"}
		issue, err := repo.issue(remote, 1)
		if err != nil || issue.WorkType != tt.want {
			t.Fatalf("%q %v: got %q, %v", tt.title, tt.labels, issue.WorkType, err)
		}
		spec := repo.spec(issue, "auto", "", validation.Policy{})
		if store.ValidWorkType(issue.WorkType) && issue.WorkType != "development" && (spec.Mode != "plan" || spec.Harness == "claude" || spec.ChildHarness != "claude") {
			t.Fatal("non-development Issue did not preserve the project harness for subsequent implementation", spec)
		}
	}
}
