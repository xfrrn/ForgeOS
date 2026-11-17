ALTER TABLE executions ADD COLUMN ci_candidate_session text;

-- Older interrupted turns may have only their final acknowledged checkpoint.
UPDATE executions e SET ci_candidate_session=(
    SELECT turn.session_id FROM execution_checkpoints turn JOIN execution_checkpoints next
      ON next.execution_id=turn.execution_id AND next.sequence=turn.sequence+1
    WHERE turn.execution_id=e.id AND turn.session_id IS NOT NULL AND next.session_id IS NULL
      AND NOT EXISTS(SELECT 1 FROM checkpoint_comments c WHERE c.execution_id=e.id AND c.sequence=next.sequence)
    ORDER BY turn.sequence DESC LIMIT 1
) WHERE e.ci_candidate IS NOT NULL AND e.uses_checkpoints AND NULLIF(e.result->>'session_id','') IS NULL;

CREATE VIEW execution_candidate_identity AS
SELECT id,task_id,requirement_id,result->>'base_commit' AS base_sha,
       COALESCE(ci_candidate,result->>'commit') AS commit_sha,
       COALESCE(NULLIF(ci_candidate_session,''),NULLIF(result->>'session_id','')) AS session_id,
       ci_resumed_at
FROM executions;

CREATE OR REPLACE FUNCTION candidate_implementation_execution(target_id bigint) RETURNS bigint
LANGUAGE sql STABLE AS $$
    WITH RECURSIVE lineage AS (
        SELECT id,task_id,requirement_id,base_sha,commit_sha,session_id
        FROM execution_candidate_identity WHERE id=target_id
        UNION ALL
        SELECT prior.id,prior.task_id,prior.requirement_id,COALESCE(prior.base_sha,l.base_sha),prior.commit_sha,prior.session_id
        FROM lineage l
        JOIN LATERAL (
            SELECT p.* FROM execution_candidate_identity p
            WHERE p.task_id=l.task_id AND p.id<l.id AND p.requirement_id IS NOT DISTINCT FROM l.requirement_id
              AND ((p.ci_resumed_at IS NOT NULL AND p.commit_sha=l.commit_sha
                    AND (p.base_sha IS NULL OR l.base_sha IS NULL OR p.base_sha=l.base_sha))
                OR EXISTS(SELECT 1 FROM delivery_refreshes f
                          WHERE f.task_id=l.task_id AND f.execution_id=p.id AND f.candidate=l.commit_sha
                            AND f.new_base=l.base_sha AND f.old_delivery->>'commit'=p.commit_sha))
            ORDER BY p.id DESC LIMIT 1
        ) prior ON l.session_id IS NULL
    )
    SELECT id FROM lineage WHERE session_id IS NOT NULL
$$;

CREATE OR REPLACE VIEW candidate_rule_context AS
SELECT e.id AS execution_id, (
    SELECT jsonb_object_agg(k,v) FROM (VALUES
        ('global_rules',to_jsonb(g)), ('decisions',NULLIF(e.decision_context,'[]'::jsonb)),
        ('acceptance_feedback',prior.feedback)
    ) fields(k,v) WHERE v IS NOT NULL
) AS context
FROM executions e
LEFT JOIN global_rule_revisions g ON g.revision=e.global_rules_revision
LEFT JOIN LATERAL (
    SELECT jsonb_build_object('execution_id',r.execution_id,'commit',r.commit,'note',r.note,'actor',r.actor) AS feedback
    FROM acceptance_reviews r JOIN executions rejected ON rejected.id=r.execution_id
    WHERE r.task_id=e.task_id AND r.execution_id<e.id AND r.decision='reject'
      AND rejected.requirement_id IS NOT DISTINCT FROM e.requirement_id
    ORDER BY r.execution_id DESC LIMIT 1
) prior ON true
WHERE COALESCE(NULLIF(e.ci_candidate_session,''),NULLIF(e.result->>'session_id','')) IS NOT NULL;

CREATE OR REPLACE FUNCTION review_rules_for_execution(target_id bigint) RETURNS jsonb
LANGUAGE sql STABLE AS $$
    SELECT c.context || CASE WHEN e.id=target_id THEN '{}'::jsonb ELSE
        jsonb_build_object('implementation_execution_id',e.id,'implementation_commit',e.commit_sha) END
    FROM execution_candidate_identity e JOIN candidate_rule_context c ON c.execution_id=e.id
    WHERE e.id=candidate_implementation_execution(target_id)
$$;
