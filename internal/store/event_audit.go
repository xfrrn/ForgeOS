package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Read immutable evidence for this event, without including the later state of its task.
func (s *Store) EventAudit(ctx context.Context, project, task, event int64) (json.RawMessage, error) {
	var receipt json.RawMessage
	err := s.db.QueryRowContext(ctx, `WITH scoped AS (
        SELECT v.*,t.project_id,t.goal_id,t.kind,e.worker,
               COALESCE(v.origin, CASE
                   WHEN v.type IN ('execution_claimed','execution_completed','execution_failed','planner_completed','ci_paused','model_paused') THEN 'worker'
                   WHEN v.type IN ('task_created','execution_abandoned','execution_retry_requested','owner_cancel_requested','ci_pause_requested','ci_resumed',
                                   'decision_answered','acceptance_approved','acceptance_rejected','plan_revised','model_pause_requested','model_resumed',
                                   'requirement_revised_from_feedback','requirement_adopted','feedback_work_item_dispatched') THEN 'owner'
                   WHEN v.type='decision_requested' AND q.id IS NOT NULL THEN
                       CASE WHEN q.checkpoint_sequence IS NULL THEN 'owner' ELSE 'worker' END
                   WHEN v.type IN ('management_reviewed','management_decision_answered','management_question_reviewed') THEN 'management'
                   WHEN v.type='source_cancel_requested' THEN 'github'
                   WHEN v.type IN ('pr_created','pr_merged','delivery_completed','goal_delivered','dependencies_delivered') THEN 'controlplane'
                   WHEN v.type IN ('delivery_base_refreshed','preview_feedback_received','feedback_screenshot_saved','feedback_recording_saved') THEN 'owner'
                   WHEN v.type='preview_feedback_classified' THEN 'management' END) AS source,
               CASE WHEN v.type IN ('execution_completed','execution_failed','execution_retry_requested','planner_completed','ci_paused')
                      OR (v.origin='worker' AND v.type IN ('owner_cancelled','source_issue_cancelled'))
                    THEN e.result END AS result,
               CASE WHEN v.type='execution_retry_requested' THEN e.abandoned_at END AS abandoned_at,
               CASE WHEN v.type IN ('ci_pause_requested','ci_paused','ci_resumed') THEN e.ci_candidate END AS ci_commit,
               CASE WHEN v.type IN ('ci_pause_requested','ci_paused','ci_resumed') THEN COALESCE(NULLIF(e.ci_candidate_session,''),
                    CASE WHEN v.type='ci_resumed' AND e.result->>'commit'=e.ci_candidate THEN NULLIF(e.result->>'session_id','') END) END AS ci_session,
               CASE WHEN v.type='management_reviewed' THEN pr.review END AS plan_review,
               CASE WHEN v.type IN ('management_question_reviewed','management_decision_answered') THEN q.management_advice END AS advice,
               CASE WHEN v.type IN ('management_question_reviewed','management_decision_answered') THEN NULLIF(q.commit_sha,'') END AS question_commit,
               CASE WHEN v.type='management_decision_answered' THEN q.reference_decision_id END AS answered_reference,
               ar.execution_id AS acceptance_id,ar.approval_basis,ar.decision,ar."commit" AS reviewed_commit,ar.actor AS reviewer,
               COALESCE(d."commit",f.old_delivery->>'commit') AS delivery_commit,
               COALESCE(d.pr_number,(f.old_delivery->>'pr_number')::bigint) AS pr_number,
               COALESCE(d.pr_url,f.old_delivery->>'pr_url') AS pr_url,
               COALESCE(d.target_branch,f.old_delivery->>'target_branch') AS target_branch,
               CASE WHEN v.type IN ('pr_merged','delivery_completed') THEN NULLIF(d.merge_sha,'') END AS merge_sha,
               CASE WHEN v.type='delivery_base_refreshed' THEN f.new_base END AS refreshed_base,
               CASE WHEN v.type='delivery_base_refreshed' THEN f.candidate END AS refreshed_candidate,
               c.goal_snapshot->'goal' AS completed_goal,
               fb.id AS feedback, fb.body->>'commit' AS feedback_commit,
               target.id AS dispatched_task,target.github_issue_id AS dispatched_issue,
               CASE WHEN v.type='preview_feedback_classified' THEN fb.classification END AS classification,
               fa.id AS artifact,fa.mime,fa.bytes,fa.sha256,
               req.id AS requirement_id,req.revision AS requirement_revision,req.issue AS issue,
               to_jsonb(cl) AS source_closure,
               mp.session_id AS pause_session,mp.resource_step AS pause_step,
               CASE WHEN v.type IN ('model_paused','model_resumed') THEN mp.index_tree END AS pause_index,
               CASE WHEN v.type IN ('model_paused','model_resumed') THEN mp.worktree_tree END AS pause_worktree,
               rp.task_id AS feedback_task,rp.execution_id AS feedback_execution,rp.commit AS revision_candidate,
               CASE WHEN rp.id IS NOT NULL THEN req.feedback_id END AS revision_feedback
        FROM task_events v JOIN work_items t ON t.id=v.task_id
        LEFT JOIN executions e ON e.id=v.execution_id AND e.task_id=t.id
        LEFT JOIN model_pauses mp ON mp.execution_id=e.id AND mp.instance=v.model_pause_instance
        LEFT JOIN github_issue_closures cl ON cl.task_id=t.source_closure_id
              AND v.type IN ('source_cancel_requested','source_issue_cancelled')
        LEFT JOIN decision_questions q ON q.id=v.question_id AND q.task_id=t.id
        LEFT JOIN plan_revisions pr ON v.type='management_reviewed' AND pr.task_id=t.id AND pr.revision=v.plan_revision
        LEFT JOIN acceptance_reviews ar ON ar.task_id=t.id AND ar.execution_id=e.id AND
              ((v.type='acceptance_approved' AND ar.decision='approve') OR (v.type='acceptance_rejected' AND ar.decision='reject'))
        LEFT JOIN deliveries d ON (d.task_id,d.execution_id)=(v.task_id,v.execution_id)
              AND v.type IN ('pr_created','pr_merged','delivery_completed')
        LEFT JOIN delivery_refreshes f ON (f.task_id,f.execution_id)=(v.task_id,v.execution_id)
              AND v.type IN ('pr_created','delivery_base_refreshed')
        LEFT JOIN material_curations c ON v.type='goal_delivered' AND c.event_id=v.id AND c.project_id=t.project_id
        LEFT JOIN preview_feedback fb ON fb.id=v.feedback_id AND EXISTS (
               SELECT 1 FROM static_previews p WHERE p.id=fb.preview_id AND p.execution_id=e.id)
        LEFT JOIN feedback_artifacts fa ON fa.id=v.feedback_artifact_id AND fa.feedback_id=fb.id
        LEFT JOIN feedback_work_items link ON v.type='feedback_work_item_dispatched' AND link.feedback_id=fb.id
        LEFT JOIN work_items target ON target.id=link.task_id AND target.project_id=t.project_id
        LEFT JOIN requirement_revisions req ON req.task_id=t.id AND
              ((v.type='issue_imported' AND req.revision=1) OR req.id=v.requirement_version_id)
        LEFT JOIN preview_feedback rf ON rf.id=req.feedback_id
        LEFT JOIN static_preview_receipts rp ON rp.id=rf.preview_id AND rp.project_id=t.project_id
        WHERE t.project_id=$1 AND t.id=$2 AND v.id=$3
    ) SELECT jsonb_build_object(
        'event_id',id,'sequence',id,'project_id',project_id,'task_id',task_id,'execution_id',execution_id,
        'session_id',COALESCE(pause_session,ci_session,NULLIF(result->>'session_id',''),plan_review->>'session_id',advice->>'session_id',classification->>'session_id'),'event_type',type,'timestamp',at,
        'actor',CASE source WHEN 'worker' THEN worker WHEN 'owner' THEN COALESCE(reviewer,'owner') WHEN 'management' THEN 'management' WHEN 'controlplane' THEN 'delivery_manager'
                            WHEN 'github' THEN CASE WHEN type IN ('issue_imported','requirement_observed','source_cancel_requested','source_issue_cancelled') THEN 'github_poller' END END,
        'source',source,'correlation_id',CASE WHEN execution_id IS NULL THEN format('TASK-%s',task_id) ELSE format('EXECUTION-%s',execution_id) END,
        'payload',jsonb_strip_nulls(jsonb_build_object(
            'goal_id',goal_id,'question_id',question_id,'plan_revision',plan_revision,'source_closure',source_closure,
            'model_pause_instance',model_pause_instance,'resource_step',pause_step,'index_tree',pause_index,'worktree_tree',pause_worktree,
            'task_kind',CASE WHEN type='task_created' THEN kind END,
            'commit',COALESCE(reviewed_commit,delivery_commit,feedback_commit,revision_candidate,ci_commit,question_commit,NULLIF(result->>'commit','')),'base_commit',COALESCE(dependency_base,NULLIF(result->>'base_commit','')),
            'abandoned_at',abandoned_at,'exit_code',result->'exit_code','harness',result->>'harness','validation_exit_code',result#>'{validation,exit_code}',
            'validation_profile',result#>>'{validation,profile}','validation_run',result#>>'{validation,run,id}',
            'decision',COALESCE(decision,plan_review->>'verdict'),'decision_level',COALESCE(plan_review->>'effective_level',advice->>'level'),
            'reference_decision_id',answered_reference,'acceptance_id',acceptance_id,'approval_basis',approval_basis,
            'pr_number',pr_number,'pr_url',pr_url,'target_branch',target_branch,'merge_sha',merge_sha,
            'new_base',refreshed_base,'new_candidate',refreshed_candidate,'completed_goal',completed_goal,
            'feedback_id',COALESCE(feedback,revision_feedback),'feedback_task_id',feedback_task,'feedback_execution_id',feedback_execution,
            'dispatched_task_id',dispatched_task,'dispatched_issue_id',dispatched_issue,
            'category',classification->>'category','artifact_id',artifact,'mime',mime,'bytes',bytes,'sha256',sha256,
            'requirement_id',requirement_id,'requirement_revision',requirement_revision,'issue_id',issue->'id',
            'issue_number',issue->'number','issue_url',NULLIF(issue->>'url',''),'repository_id',issue->'repository_id',
            'work_type',NULLIF(issue->>'work_type',''))))
        FROM scoped`, project, task, event).Scan(&receipt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return receipt, err
}
