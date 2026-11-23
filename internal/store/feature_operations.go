package store

import (
	"context"
	"database/sql"
	"errors"
)

const FeatureCancelled = "本轮已取消。"
const FeatureCancelUnconfirmed = "已停止等待回复。原生客户端的取消结果未能核实。"
const FeatureSessionMissing = "原生会话已丢失。可从本话题记录恢复并重新提问。"

var errDiscussionBusy = errors.New("discussion model slots busy")

func reserveDiscussionSlot(ctx context.Context, tx *sql.Tx, turn int64, worker, harness string) error {
	if _, err := tx.ExecContext(ctx, "LOCK TABLE model_budgets IN ROW SHARE MODE"); err != nil {
		return err
	}
	var limit, used int
	err := tx.QueryRowContext(ctx, "SELECT coalesce(model_slots,slots) FROM worker_nodes WHERE worker=$1 FOR UPDATE", worker).Scan(&limit)
	if errors.Is(err, sql.ErrNoRows) {
		return errDiscussionBusy
	}
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT u.model_reserved FROM worker_nodes n CROSS JOIN LATERAL (`+workerUsage+`) u WHERE n.worker=$1`, worker).Scan(&used)
	if err != nil {
		return err
	}
	if used >= limit {
		return errDiscussionBusy
	}
	pool, available, err := modelSlot(ctx, tx, harness, worker, "", 0)
	if err != nil {
		return err
	}
	if !available {
		return errDiscussionBusy
	}
	_, err = tx.ExecContext(ctx, "UPDATE project_chat_turns SET resource_worker=$2, model_budget=$3 WHERE id=$1", turn, worker, pool)
	return err
}
func (s *Store) CancelFeatureTurn(ctx context.Context, project, feature, turn int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE project_chat_turns SET cancel_requested=true,
 status=CASE WHEN status='queued' THEN 'failed' ELSE status END,
 error=CASE WHEN status='queued' THEN $4 ELSE error END,
 answer=CASE WHEN status IN ('queued','running') THEN '' ELSE answer END
 WHERE project_id=$1 AND feature_id=$2 AND id=$3 AND (status IN ('queued','running') OR cancel_requested)`, project, feature, turn, FeatureCancelled)
	return chatUpdated(res, err)
}
func (s *Store) FeatureCancelRequested(ctx context.Context, project, turn int64, lease string) (bool, error) {
	var stop bool
	err := s.db.QueryRowContext(ctx, `SELECT cancel_requested FROM project_chat_turns WHERE project_id=$1 AND id=$2 AND feature_id IS NOT NULL AND status='running' AND lease=$3 AND lease_until>now()`, project, turn, lease).Scan(&stop)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrConflict
	}
	return stop, err
}
func (s *Store) RecoverFeatureTurn(ctx context.Context, project, feature, turn int64) (ChatTurn, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO project_chat_turns(project_id,feature_id,client_id,text,requested_model,requested_harness,model_profile_id,model_revision,model_profile_name,native_client,recovery_of)
 SELECT t.project_id,t.feature_id,gen_random_uuid(),t.text,t.requested_model,t.requested_harness,t.model_profile_id,t.model_revision,t.model_profile_name,t.native_client,t.id
 FROM project_chat_turns t JOIN feature_clients c ON c.id=t.native_client WHERE t.project_id=$1 AND t.feature_id=$2 AND t.id=$3 AND t.proposal_id IS NULL AND t.session_missing
 AND c.seen_at>now()-interval '3 minutes' AND NOT EXISTS(SELECT 1 FROM project_chat_turns newer WHERE newer.project_id=$1 AND newer.feature_id=$2 AND newer.native_client=t.native_client AND newer.id>t.id AND newer.recovery_of IS NOT NULL)
 ON CONFLICT DO NOTHING RETURNING id`, project, feature, turn).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, "SELECT id FROM project_chat_turns WHERE project_id=$1 AND feature_id=$2 AND recovery_of=$3", project, feature, turn).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrConflict
	}
	if err != nil {
		return ChatTurn{}, err
	}
	return s.ChatTurn(ctx, project, id)
}
