package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestCentralArchiveResumesVerifiesAndExpires(t *testing.T) {
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
			t.Fatal("expected conflict", err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, "archived")
	must(err)
	var task, execution int64
	must(s.db.QueryRow("INSERT INTO work_items(project_id,status) VALUES($1,'DONE') RETURNING id", p.ID).Scan(&task))
	line := append(bytes.Repeat([]byte("x"), ProcessArchiveChunkSize+31), '\n')
	suffix := []byte("tail\n")
	first := fmt.Sprintf("%x", sha256.Sum256(line))
	a := ProcessArchive{Records: 2, Bytes: int64(len(line) + len(suffix)), Digest: fmt.Sprintf("%x", sha256.Sum256(append([]byte(first), suffix...))), LimitBytes: 1 << 20, Central: true}
	raw, err := json.Marshal(map[string]any{"processes": []any{map[string]any{"instance": 12, "archive": a}}})
	must(err)
	must(s.db.QueryRow("INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until,result) VALUES($1,'node','work','token',now(),$2) RETURNING id", task, string(raw)).Scan(&execution))
	_, err = s.db.Exec("INSERT INTO process_archive_transfers(execution_id,instance,expires_at) VALUES($1,12,now()+interval '1 day')", execution)
	must(err)
	content := append(line, suffix...)
	put := func(offset int64, b []byte) error { return s.PutArchiveChunk(ctx, task, execution, 12, offset, b) }
	remainder := content[ProcessArchiveChunkSize:]
	block := content[:ProcessArchiveChunkSize]
	conflict(put(ProcessArchiveChunkSize, remainder))
	must(put(0, block))
	must(put(0, block))
	bad := bytes.Clone(block)
	bad[0]++
	conflict(put(0, bad))
	if err = s.DownloadProcessArchive(ctx, task, execution, 12, func(ProcessArchive) io.Writer { t.Fatal("partial emitted"); return nil }); !errors.Is(err, ErrProcessArchivePending) {
		t.Fatal(err)
	}
	inconsistent := bytes.Clone(remainder)
	inconsistent[0]++
	conflict(put(ProcessArchiveChunkSize, inconsistent))
	status, err := s.ArchiveUploadStatus(ctx, task, execution, 12)
	must(err)
	if status.Received != 0 || status.Verified {
		t.Fatal(status)
	}
	must(put(0, block))
	must(put(ProcessArchiveChunkSize, remainder))
	must(put(ProcessArchiveChunkSize, remainder))
	var downloaded bytes.Buffer
	must(s.DownloadProcessArchive(ctx, task, execution, 12, func(got ProcessArchive) io.Writer {
		if got != a {
			t.Fatal(got)
		}
		return &downloaded
	}))
	if !bytes.Equal(downloaded.Bytes(), content) {
		t.Fatal("download differs")
	}
	_, err = s.ArchiveUploadStatus(ctx, task+1, execution, 12)
	if !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	_, err = s.db.Exec("UPDATE process_archive_chunks SET content=$1 WHERE execution_id=$2 AND start_byte=0", bad, execution)
	must(err)
	conflict(s.DownloadProcessArchive(ctx, task, execution, 12, func(ProcessArchive) io.Writer { t.Fatal("corrupt bytes emitted"); return nil }))
	_, err = s.db.Exec("UPDATE process_archive_transfers SET expires_at=now() WHERE execution_id=$1", execution)
	must(err)
	if err = put(0, block); !errors.Is(err, ErrProcessArchiveExpired) {
		t.Fatal(err)
	}
	must(s.PurgeProcessArchiveChunks(ctx))
	var n int
	must(s.db.QueryRow("SELECT count(*) FROM process_archive_chunks WHERE execution_id=$1", execution).Scan(&n))
	if n != 0 {
		t.Fatal("expired content remains")
	}
}
