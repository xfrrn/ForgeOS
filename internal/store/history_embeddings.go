package store

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/lib/pq"
)

// Scope before encoding. Identical text from two revisions of one task can share a vector.
func (s *Store) indexHistory(ctx context.Context, project, task int64, source string, dimensions int) error {
	type document struct {
		task int64
		text string
	}
	for {
		rows, err := s.db.QueryContext(ctx, historyRecords+`
			SELECT DISTINCT r.task_id,r.content FROM records r WHERE r.content IS NOT NULL AND btrim(r.content)<>''
			AND NOT EXISTS (SELECT 1 FROM history_embeddings e WHERE e.task_id=r.task_id
				AND e.content_hash=sha256(convert_to(r.content,'UTF8')) AND e.model=$4)
			ORDER BY r.task_id,r.content LIMIT 16`, project, task, source, s.embeddings.model)
		if err != nil {
			return err
		}
		var batch []document
		for rows.Next() {
			var d document
			if err = rows.Scan(&d.task, &d.text); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, d := range batch {
			v, err := s.embeddings.textVector(ctx, d.text)
			if err != nil {
				return err
			}
			if len(v) != dimensions {
				return errors.New("history embedding dimensions differ from query")
			}
			digest := sha256.Sum256([]byte(d.text))
			_, err = s.db.ExecContext(ctx, `INSERT INTO history_embeddings(task_id,content_hash,model,embedding)
				VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, d.task, digest[:], s.embeddings.model, pq.Array(v))
			if err != nil {
				return err
			}
		}
	}
}
