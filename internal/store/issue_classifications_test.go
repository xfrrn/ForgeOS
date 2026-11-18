package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIssueClassificationRequiresCurrentSourceProcessAndBudget(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	conflict := func(err error) {
		t.Helper()
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("wanted conflict, got %v", err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, "intake")
	must(err)
	issue := Issue{ID: 11, Number: 1, RepositoryID: 80, WorkType: "clarification", Title: "Queue preview", Body: "We need tests."}
	entry := Discussion{ID: issue.ID, Number: issue.Number, Title: issue.Title, WorkType: issue.WorkType, SourceDigest: strings.Repeat("a", 64), ClassificationSource: &issue}
	other := entry
	other.ID++
	other.Number++
	source := issue
	source.ID = other.ID
	source.Number = other.Number
	other.ClassificationSource = &source
	snapshot := func() { must(s.ReplaceInbox(ctx, p.ID, 80, []Discussion{entry, other})) }
	snapshot()
	claim := func() *ManagementJob { t.Helper(); j, e := s.ClaimIssueClassification(ctx, p.ID); must(e); return j }
	job := claim()
	if job == nil || job.IssueClassification.Source != issue || job.LeaseUntil == nil {
		t.Fatal(job)
	}
	must(s.SetModelBudgets(ctx, []ModelBudget{{Name: "account", Harness: "codex", Slots: 1, AgentRoles: []ModelBudgetRole{{Project: p.ID, Role: "issue_classification"}}}}))
	if claim() != nil {
		t.Fatal("running classifier did not reserve a slot")
	}
	profile := StructuredCodexProfile()
	suggestion := IssueSuggestion{WorkType: "testing", Quote: "We need tests.", Rationale: "The deliverable is a test.", Session: "codex-thread", Profile: &profile}
	save := func(v IssueSuggestion) error {
		return s.SuggestIssueType(ctx, p.ID, job.IssueClassification.ID, job.Lease, v)
	}
	conflict(save(suggestion))
	end := time.Now().UTC()
	code := 0
	report := ManagementProcessReport{IssueClassificationID: job.IssueClassification.ID, Lease: job.Lease, Node: "classifier", Process: HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.0.0", Session: suggestion.Session, Started: end.Add(-time.Second), Exited: &end, ExitCode: &code}}
	conflict(s.ObserveManagementProcess(ctx, p.ID+1, report))
	must(s.ObserveManagementProcess(ctx, p.ID, report))
	manifest := ProcessArchive{Records: 1, Bytes: 120, LimitBytes: 1 << 20, Digest: strings.Repeat("b", 64)}
	archived := report
	archived.Process.Archive = &manifest
	must(s.ObserveManagementProcess(ctx, p.ID, archived))
	must(s.ObserveManagementProcess(ctx, p.ID, archived))
	changed := manifest
	changed.Bytes++
	archived.Process.Archive = &changed
	conflict(s.ObserveManagementProcess(ctx, p.ID, archived))
	changed = manifest
	changed.Central = true
	conflict(s.ObserveManagementProcess(ctx, p.ID, archived))
	must(s.ObserveManagementProcess(ctx, p.ID, report))
	checkHistory := func(settled bool) {
		t.Helper()
		raw, err := s.IssueClassificationProcesses(ctx, p.ID, report.IssueClassificationID)
		must(err)
		var h struct {
			Settled   bool
			Processes []ManagementProcess
		}
		must(json.Unmarshal(raw, &h))
		if len(h.Processes) != 1 || h.Processes[0].Archive == nil || *h.Processes[0].Archive != manifest {
			t.Fatal("node archive changed on replay", h)
		}
		if h.Settled != settled || len(h.Processes) != 1 || h.Processes[0].Node != report.Node || h.Processes[0].Session != report.Process.Session || h.Processes[0].Attempt < 1 || strings.Contains(string(raw), job.Lease) {
			t.Fatal("invalid classification history", h)
		}
	}
	checkHistory(false)
	if _, err := s.IssueClassificationProcesses(ctx, p.ID+1, report.IssueClassificationID); !errors.Is(err, ErrNotFound) {
		t.Fatal("wrong project", err)
	}
	upload := report
	upload.Process.Archive = &manifest
	remote, err := s.RegisterRoleArchive(ctx, p.ID, upload)
	must(err)
	if remote < 1 {
		t.Fatal("missing remote process")
	}
	again, err := s.RegisterRoleArchive(ctx, p.ID, upload)
	must(err)
	if again != remote {
		t.Fatal("replay created another archive")
	}
	ownerTarget := ArchiveTarget{RoleProcess: remote, Instance: 1}
	scopedTarget := ownerTarget
	scopedTarget.Role, scopedTarget.Project = "management", p.ID
	a, err := s.ArchiveUploadStatus(ctx, scopedTarget)
	must(err)
	if !a.Central || a.Received != 0 {
		t.Fatal(a)
	}
	for _, wrong := range []ArchiveTarget{
		{RoleProcess: remote, Instance: 1, Role: "curator", Project: p.ID},
		{RoleProcess: remote, Instance: 1, Role: "management", Project: p.ID + 1},
		{RoleProcess: remote, Instance: 2, Role: "management", Project: p.ID},
	} {
		if _, err = s.ArchiveUploadStatus(ctx, wrong); !errors.Is(err, ErrNotFound) {
			t.Fatal("unexpected archive access", err)
		}
		if err = s.PutArchiveChunk(ctx, wrong, 0, []byte("x")); !errors.Is(err, ErrNotFound) {
			t.Fatal("unexpected archive write", err)
		}
	}
	must(s.ObserveManagementProcess(ctx, p.ID, report))
	manifest.Central = true
	bad := suggestion
	bad.Quote = "unsupported citation"
	conflict(save(bad))
	bad = suggestion
	bad.Session = "wrong-thread"
	conflict(save(bad))
	entry.SourceDigest = strings.Repeat("b", 64)
	snapshot()
	conflict(save(suggestion))
	entry.SourceDigest = strings.Repeat("a", 64)
	snapshot()
	must(save(suggestion))
	must(save(suggestion))
	checkHistory(true)
	manifest.Central = false
	replayed, err := s.RegisterRoleArchive(ctx, p.ID, upload)
	must(err)
	if replayed != remote {
		t.Fatal("lost archive ID after settlement")
	}
	_, err = s.RegisterRoleArchive(ctx, p.ID+1, upload)
	conflict(err)
	wrongNode := upload
	wrongNode.Node = "another"
	_, err = s.RegisterRoleArchive(ctx, p.ID, wrongNode)
	conflict(err)
	wrongManifest := manifest
	wrongManifest.Bytes++
	upload.Process.Archive = &wrongManifest
	_, err = s.RegisterRoleArchive(ctx, p.ID, upload)
	conflict(err)
	bad = suggestion
	bad.WorkType = "operations"
	conflict(save(bad))
	if claim() == nil {
		t.Fatal("completed classifier held the slot")
	}
	items, err := s.Inbox(ctx, p.ID, 0)
	must(err)
	if len(items) != 2 || items[1].ClassificationID != report.IssueClassificationID || items[1].Suggestion == nil || items[1].Suggestion.Session != suggestion.Session {
		t.Fatal(items)
	}
	entry.ClassificationSource = nil
	snapshot()
	items, err = s.Inbox(ctx, p.ID, 0)
	must(err)
	if items[1].Suggestion != nil || items[1].ClassificationID != 0 {
		t.Fatal("ordinary discussion exposed a historical suggestion")
	}
	entry.ClassificationSource = &issue
	snapshot()
	issue.ClassificationDigest = entry.SourceDigest
	issue.OwnerClassified = true
	issue.WorkType = "investigation"
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: issue, BaseCommit: strings.Repeat("a", 40), Validation: "docs", Mode: "plan"})
	must(err)
	revision, err := s.RequirementRevision(ctx, task.ID, 1)
	must(err)
	if revision.ClassificationID != report.IssueClassificationID || revision.Suggestion == nil || revision.Suggestion.WorkType != "testing" || revision.Issue.WorkType != "investigation" {
		t.Fatal(revision)
	}
}
