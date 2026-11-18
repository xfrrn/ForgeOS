package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

func TestCandidateFinalizationRecovery(t *testing.T) {
	for _, phase := range []string{"before-head", "after-head", "validated", "changed-worktree", "expired-lease", "cancelled", "save-failed", "validation-failed", "receipt-failed", "owner-stop", "source-close"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			stopped := phase == "cancelled" || phase == "owner-stop" || phase == "source-close"
			w, job, base := fixture(t)
			job.Kind, job.LeaseSeconds, job.ManageResources, job.ReportCICandidate = "codex_issue", 300, true, true
			job.Spec = &store.IssueSpec{BaseCommit: base, Validation: "docs"}
			w.codex = "must-not-start-a-model"
			dir, err := checkedWorktree(ctx, job.RepoPath, job.Workspace, base)
			if err != nil {
				t.Fatal(err)
			}
			filename := "README.md"
			if phase == "validation-failed" {
				filename = "source.go"
			}
			if err = os.WriteFile(filepath.Join(job.Workspace, filename), []byte("Completed by the original model.\n"), 0600); err != nil {
				t.Fatal(err)
			}

			claims, releases, reports := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/renew"):
					if phase == "expired-lease" {
						rw.WriteHeader(409)
						return
					}
					if phase == "cancelled" {
						rw.WriteHeader(423)
						return
					}
				case strings.HasSuffix(r.URL.Path, "/ci-candidate"):
				case strings.HasSuffix(r.URL.Path, "/resource"):
					var input store.ResourceRequest
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Step != 2 || input.Kind != "ci" {
						t.Error("wrong reservation", err)
					}
					if input.Release {
						releases++
					} else {
						claims++
						if phase == "owner-stop" {
							cancel(store.ErrOwnerStop)
						}
						if phase == "source-close" {
							cancel(store.ErrSourceClosed)
						}
					}
				case strings.HasSuffix(r.URL.Path, "/result"):
					var result store.Result
					if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
						t.Error(err)
					}
					if phase == "validation-failed" && (result.ExitCode == 0 || result.Validation == nil || result.Validation.ExitCode == 0) {
						t.Error("failed validation was lost")
					}
					if !stopped && phase != "validation-failed" && (result.ExitCode != 0 || result.Validation == nil || result.Validation.Commit != result.Commit || result.SessionID != "original-session") {
						t.Error("lost candidate result")
					}
					reports++
				default:
					t.Error("unexpected request", r.URL.Path)
					rw.WriteHeader(400)
					return
				}
				rw.WriteHeader(204)
			}))
			defer server.Close()
			w.api, w.client = server.URL, server.Client()
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			w.journal, err = w.openJournal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer w.journal.Close()
			key := sha256.Sum256([]byte(job.LeaseToken))
			if _, err = w.journal.Exec("INSERT INTO attempts(lease_hash,execution_id,task_id,workspace) VALUES(?,?,?,?)", key[:], job.ExecutionID, job.TaskID, job.Workspace); err != nil {
				t.Fatal(err)
			}
			q := parkedCheckpoint{Job: job, GitDir: dir, Session: "original-session", Remaining: time.Minute, ResourceStep: 1}
			_, err = commitChangesCheckpoint(ctx, job.Workspace, base, base, 1, dir, func(tip, candidate string) error {
				q.Finalization = &candidateFinalization{Tip: tip, Result: store.Result{BaseCommit: base, Commit: candidate, SessionID: q.Session}}
				if err := w.saveFinalization(ctx, &q); err != nil {
					return err
				}
				if phase == "before-head" {
					return errors.New("simulated Daemon exit before HEAD update")
				}
				return nil
			})
			if err != nil && phase != "before-head" {
				t.Fatal(err)
			}
			if phase == "before-head" && git(t, job.Workspace, "rev-parse", "HEAD") != base {
				t.Fatal("candidate attached before snapshot")
			}
			if phase == "changed-worktree" {
				os.WriteFile(filepath.Join(job.Workspace, "README.md"), []byte("External edit\n"), 0600)
			}
			if phase == "validated" {
				q.Finalization.Validated = true
				q.Finalization.Result.Validation = &store.ValidationResult{Profile: "docs", Commit: q.Finalization.Result.Commit, Output: "Original validated result"}
				if err := w.saveFinalization(ctx, &q); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "save-failed" {
				if _, err := w.journal.Exec("CREATE TRIGGER fail_candidate_save BEFORE UPDATE ON candidate_finalizations BEGIN SELECT RAISE(FAIL,'disk fault'); END"); err != nil {
					t.Fatal(err)
				}
			}
			// Reopen the actual journal as a replacement Daemon.
			if err = w.journal.Close(); err != nil {
				t.Fatal(err)
			}
			w.journal, err = w.openJournal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer w.journal.Close()
			ids, err := w.finalizingExecutions(ctx)
			if err != nil || len(ids) != 1 || ids[0] != job.ExecutionID {
				t.Fatal(ids, err)
			}
			saved, err := w.readFinalization(ctx, job.ExecutionID)
			if err != nil || saved == nil {
				t.Fatal(err)
			}
			if phase == "receipt-failed" {
				if _, err := w.journal.Exec("CREATE TRIGGER fail_receipt BEFORE UPDATE OF buffered_at ON attempts BEGIN SELECT RAISE(FAIL,'receipt fault'); END"); err != nil {
					t.Fatal(err)
				}
				if err := w.runAttempt(ctx, saved.Job, time.Now(), saved); err == nil {
					t.Fatal("receipt fault ignored")
				}
				if reports != 0 || claims != 1 || releases != 1 {
					t.Fatal("unexpected pre-restart side effects")
				}
				saved, err = w.readFinalization(ctx, job.ExecutionID)
				if err != nil || saved == nil || !saved.Finalization.Validated {
					t.Fatal("validated result lost", err)
				}
				if _, err = w.journal.Exec("DROP TRIGGER fail_receipt"); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.runAttempt(ctx, saved.Job, time.Now(), saved); err != nil {
				t.Fatal(err)
			}
			blocked := phase == "changed-worktree" || phase == "expired-lease" || phase == "save-failed"
			if blocked {
				if reports != 0 || releases != 0 {
					t.Fatal("unconfirmed candidate was settled or released")
				}
				if kept, err := w.readFinalization(ctx, job.ExecutionID); err != nil || kept == nil || kept.Finalization.Validated {
					t.Fatal("lost recovery evidence", err)
				}
				return
			}
			if reports != 1 {
				t.Fatal("result not reported exactly once", reports)
			}
			expectedReleases := 1
			if phase == "receipt-failed" {
				expectedReleases = 2
			}
			if !stopped && (releases != expectedReleases || (phase == "validated" && claims != 0) || (phase != "validated" && claims != 1)) {
				t.Fatal(claims, releases)
			}
			if kept, err := w.readFinalization(context.Background(), job.ExecutionID); err != nil || kept != nil {
				t.Fatal("settled candidate retained private snapshot", err)
			}
			if !stopped && git(t, job.Workspace, "rev-parse", "HEAD") != q.Finalization.Result.Commit {
				t.Fatal("recovery created another SHA")
			}
		})
	}
}

func TestExecutionJournalLock(t *testing.T) {
	w, _, _ := fixture(t)
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var err error
	w.journal, err = w.openJournal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	unlock, err := w.lockAttempt(1)
	if err != nil {
		t.Fatal(err)
	}
	if release, err := w.lockAttempt(1); err == nil {
		release()
		t.Fatal("second execution owner admitted")
	}
	other, err := w.lockAttempt(2)
	if err != nil {
		t.Fatal("unrelated execution blocked", err)
	}
	other()
	unlock()
	unlock, err = w.lockAttempt(1)
	if err != nil {
		t.Fatal("lock not released", err)
	}
	unlock()
}
