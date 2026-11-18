package store

import (
	"context"
	"encoding/json"
)

type Discussion struct {
	SourceDigest string `json:"source_digest,omitempty"`
	WorkType     string `json:"work_type,omitempty"`
	ID           int64  `json:"id"`
	Number       int64  `json:"number"`
	Title        string `json:"title"`
	URL          string `json:"url"`
}

// ReplaceInbox publishes only a complete scan so a failed page cannot erase discussions.
func (s *Store) ReplaceInbox(ctx context.Context, projectID, repositoryID int64, entries []Discussion) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = bindRepository(ctx, tx, projectID, repositoryID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM discussion_inbox WHERE project_id=$1`, projectID); err != nil {
		return err
	}
	if len(entries) > 0 {
		data, err := json.Marshal(entries)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO discussion_inbox(project_id,issue_id,number,title,url,work_type,source_digest)
			SELECT $1,id,number,title,url,work_type,source_digest FROM jsonb_to_recordset($2::jsonb)
			AS i(id bigint,number bigint,title text,url text,work_type text,source_digest text) ON CONFLICT DO NOTHING`, projectID, string(data))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Inbox(ctx context.Context, projectID, before int64) ([]Discussion, error) {
	if _, err := s.Project(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT issue_id,number,title,url,COALESCE(work_type,''),COALESCE(source_digest,'') FROM discussion_inbox d
		WHERE project_id=$1 AND ($2::bigint=0 OR number<$2)
		AND NOT EXISTS (SELECT 1 FROM work_items w WHERE w.project_id=d.project_id AND w.github_issue_id=d.issue_id)
		ORDER BY number DESC LIMIT 100`, projectID, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Discussion{}
	for rows.Next() {
		var item Discussion
		if err := rows.Scan(&item.ID, &item.Number, &item.Title, &item.URL, &item.WorkType, &item.SourceDigest); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
