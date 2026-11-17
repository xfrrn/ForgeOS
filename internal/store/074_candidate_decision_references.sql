CREATE OR REPLACE VIEW candidate_rule_context AS
SELECT e.id AS execution_id, (
    SELECT jsonb_object_agg(k,v) FROM (VALUES
        ('global_rules',to_jsonb(g)), ('decisions',NULLIF(e.decision_context,'[]'::jsonb)),
        ('acceptance_feedback',prior.feedback),
        ('management_references',(
            SELECT jsonb_agg(jsonb_build_object('question_id',q.id,'sequence',q.checkpoint_sequence,
                'decision',q.reference_snapshot,'session_id',q.management_advice->>'session_id')
                ORDER BY q.checkpoint_sequence)
            FROM decision_questions q WHERE q.execution_id=e.id AND q.answered_by='management'
                AND q.reference_snapshot IS NOT NULL
        ))
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
