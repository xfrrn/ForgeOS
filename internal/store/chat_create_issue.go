package store

import "context"

// Commit the intent before sending a non-idempotent remote POST. Only its winner
// may send it; concurrent confirms and restarts reconcile the same marker.
func (s *Store) BeginChatIssue(ctx context.Context, project, id int64, title, body, workType string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE project_chat_actions SET create_started_at=now(),label=$3,issue_body=$4,work_type=$5
        WHERE project_id=$1 AND id=$2 AND status='pending' AND kind='create_issue' AND create_started_at IS NULL AND issue_number=0`, project, id, title, body, workType)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// Only definitive rejections (including missing credentials before POST) are retryable as a new POST.
func (s *Store) RejectChatIssue(ctx context.Context, project, id int64) error {
	return chatUpdated(s.db.ExecContext(ctx, `UPDATE project_chat_actions SET create_started_at=NULL
        WHERE project_id=$1 AND id=$2 AND kind='create_issue' AND status='pending' AND issue_number=0`, project, id))
}

func (s *Store) RecordChatIssue(ctx context.Context, project, id, number int64) error {
	return chatUpdated(s.db.ExecContext(ctx, `UPDATE project_chat_actions SET issue_number=$3
        WHERE project_id=$1 AND id=$2 AND kind='create_issue' AND create_started_at IS NOT NULL AND issue_number IN (0,$3)`, project, id, number))
}
