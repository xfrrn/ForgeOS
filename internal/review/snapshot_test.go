package review

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
)

func TestReviewChecksEvidenceAgainstBothGitObjects(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := localrepo.Git(ctx, repo, args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	path := "queue sample.txt"
	save := func(text string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, path), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		git("add", "--", path)
		git("-c", "user.name=QA", "-c", "user.email=qa@local", "commit", "-m", "update")
		return git("rev-parse", "HEAD")
	}
	base := save("old\n")
	head := save("new\nnext\n")
	if err := os.WriteFile(filepath.Join(repo, path), []byte("dirty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Read(ctx, repo, base, head)
	if err != nil || len(s.Files) != 1 {
		t.Fatal(s, err)
	}
	f := Finding{ID: "F1", Severity: "Medium", File: path, Side: "candidate", Line: 2, Evidence: "next", Description: "description", Recommendation: "recommendation", Confidence: .9}
	r := Report{Session: "native-1", Base: base, Commit: head, Summary: "reviewed", Findings: []Finding{f}}
	for _, tc := range []struct {
		side     string
		line     int
		text     string
		verified bool
	}{
		{"candidate", 2, "next", true}, {"base", 1, "old", true}, {"candidate", 1, "dirty", false}, {"base", 2, "next", false},
	} {
		r.Findings[0].Side, r.Findings[0].Line, r.Findings[0].Evidence = tc.side, tc.line, tc.text
		if err := s.Verify(&r); err != nil || r.Findings[0].EvidenceVerified != tc.verified {
			t.Fatal(r.Findings, err)
		}
	}
	profile := ClaudeProfile()
	r.Profile = &profile
	if err := s.Verify(&r); err != nil {
		t.Fatal(err)
	}
	for _, tools := range []*[]string{nil, {"Read"}} {
		profile.Tools = tools
		if s.Verify(&r) == nil {
			t.Fatal("accepted an incomplete or expanded reviewer tool policy")
		}
	}
	r.Profile = nil
	r.Findings = append(r.Findings, r.Findings[0])
	if s.Verify(&r) == nil {
		t.Fatal("accepted duplicate finding IDs")
	}
	binary := save("binary\x00data")
	if _, err = Read(ctx, repo, head, binary); err == nil {
		t.Fatal("review silently omitted a binary change")
	}
}
