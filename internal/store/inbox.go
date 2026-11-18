package store

import (
	"context"
	"encoding/json"
)

type Discussion struct {
	ClassificationSource *Issue           `json:"-"`
	Suggestion           *IssueSuggestion `json:"suggestion,omitempty"`
	SourceDigest         string           `json:"source_digest,omitempty"`
	WorkType             string           `json:"work_type,omitempty"`
	ID                   int64            `json:"id"`
	Number               int64            `json:"number"`
	Title                string           `json:"title"`
	URL                  string           `json:"url"`
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
		type snapshot struct {
			Discussion
			Source *Issue `json:"source,omitempty"`
		}
		items := make([]snapshot, len(entries))
		for i, e := range entries {
			items[i] = snapshot{e, e.ClassificationSource}
		}
		data, err := json.Marshal(items)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `WITH items AS (
			SELECT * FROM jsonb_to_recordset($2::jsonb) AS i(id bigint,number bigint,title text,url text,work_type text,source_digest text,source jsonb)
		), published AS (
			INSERT INTO discussion_inbox(project_id,issue_id,number,title,url,work_type,source_digest,classification_requested)
			SELECT $1,id,number,title,url,work_type,source_digest,source IS NOT NULL FROM items ON CONFLICT DO NOTHING
			RETURNING issue_id,source_digest,classification_requested
		) INSERT INTO issue_classifications(project_id,issue_id,source_digest,source)
		  SELECT $1,i.id,i.source_digest,i.source FROM published p JOIN items i ON i.id=p.issue_id AND i.source_digest=p.source_digest
		  WHERE p.classification_requested ON CONFLICT DO NOTHING`, projectID, string(data))
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
	rows, err := s.db.QueryContext(ctx, `SELECT d.issue_id,d.number,d.title,d.url,COALESCE(d.work_type,''),COALESCE(d.source_digest,''),CASE WHEN d.classification_requested THEN c.suggestion END FROM discussion_inbox d
		LEFT JOIN issue_classifications c USING(project_id,issue_id,source_digest)
		WHERE d.project_id=$1 AND ($2::bigint=0 OR number<$2)
		AND NOT EXISTS (SELECT 1 FROM work_items w WHERE w.project_id=d.project_id AND w.github_issue_id=d.issue_id)
		ORDER BY number DESC LIMIT 100`, projectID, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Discussion{}
	for rows.Next() {
		var item Discussion
		var raw []byte
		if err := rows.Scan(&item.ID, &item.Number, &item.Title, &item.URL, &item.WorkType, &item.SourceDigest, &raw); err != nil {
			return nil, err
		}
		if raw != nil {
			if err := json.Unmarshal(raw, &item.Suggestion); err != nil {
				return nil, err
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
