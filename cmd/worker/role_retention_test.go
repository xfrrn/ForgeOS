package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoleRecoveryRetention(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	t.Setenv("FORGEOS_WORKER_STATE_DIR", dir)
	w := worker{id: "retention"}
	db, _, err := w.openRoleJournal(context.Background(), "management")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir, err = roleJournalDirectory(db)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.MkdirTemp(dir, "role-management-")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(workspace, "output.json")
	if err := os.WriteFile(output, []byte("completed turn"), 0600); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO harness_processes(instance,lease_hash,harness,version,pid,exited_at) VALUES(1,x'01','codex','1',9,'2000-01-01')")
	exec("INSERT INTO role_native_turns VALUES(x'01',1,'codex','1',?,'saved input','saved schema',1)", workspace)
	exec("INSERT INTO role_receipts(lease_hash,path,digest,status,settled_at) VALUES(x'01','/internal/management/result',x'02',204,CURRENT_TIMESTAMP)")
	check := func(count int, wantErr bool) {
		t.Helper()
		got, err := pruneRoleRecovery(db, time.Now().Add(-24*time.Hour))
		if got != count || (err != nil) != wantErr {
			t.Fatalf("got count %d error %v, want %d error %v", got, err, count, wantErr)
		}
	}
	check(0, false)
	exec("UPDATE role_receipts SET settled_at='2000-01-01'")
	exec("INSERT INTO role_assignment VALUES(1,'k',x'01','run',x'00',1,CURRENT_TIMESTAMP)")
	check(0, false)
	exec("DELETE FROM role_assignment")
	exec("INSERT INTO role_turn_results(lease_hash,turn) VALUES(x'01',1)")
	check(0, false)
	exec("DELETE FROM role_turn_results")
	exec("INSERT INTO role_process_receipts(instance,report) VALUES(1,x'00')")
	check(0, false)
	exec("UPDATE role_process_receipts SET settled_at=CURRENT_TIMESTAMP,status=204,report=NULL")
	check(0, false)
	exec("UPDATE role_process_receipts SET settled_at='2000-01-01',status=204,report=NULL")
	exec("INSERT INTO harness_processes(instance,lease_hash,harness,version,pid) VALUES(2,x'01','codex','1',10)")
	check(0, false)
	exec("UPDATE harness_processes SET exited_at='2000-01-01' WHERE instance=2")
	unknown := filepath.Join(workspace, "owner-notes")
	if err := os.WriteFile(unknown, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	check(0, true)
	if _, err := os.Stat(output); err != nil {
		t.Fatal("rejected directory was partially cleared", err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	check(1, false)
	check(0, false)
	var cleared bool
	if err := db.QueryRow("SELECT prompt IS NULL AND contract IS NULL AND instance=1 AND workspace=? FROM role_native_turns", workspace).Scan(&cleared); err != nil || !cleared {
		t.Fatal("lost metadata or kept frozen input", err)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatal("expired workspace retained", err)
	}
}

func TestRoleRecoveryRemovalStaysWithinJournal(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "state")
	other := filepath.Join(root, "role-management-123")
	for _, p := range []string{dir, other} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeRoleRecoveryFiles(dir, other, "management"); err == nil {
		t.Fatal("accepted another journal's path")
	}
	if err := removeRoleRecoveryFiles(root, other, "curator"); err == nil {
		t.Fatal("accepted a different role")
	}
	link := filepath.Join(dir, "role-management-19")
	if err := os.Symlink(other, link); err == nil {
		if err := removeRoleRecoveryFiles(dir, link, "management"); err == nil {
			t.Fatal("accepted linked workspace")
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("outside directory was removed", err)
	}
	file := filepath.Join(other, "external.json")
	if err := os.WriteFile(file, []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(dir, "role-management-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, filepath.Join(work, "schema.json")); err == nil {
		if err := removeRoleRecoveryFiles(dir, work, "management"); err == nil {
			t.Fatal("accepted linked recovery file")
		}
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "preserved" {
		t.Fatal("outside file changed", err)
	}
}
