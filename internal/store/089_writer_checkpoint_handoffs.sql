ALTER TABLE model_pauses ADD COLUMN handoff_sequence integer,
    ADD COLUMN handoff_instance bigint, ADD COLUMN handoff_session text,
    ADD CONSTRAINT model_pause_handoff CHECK (
        (handoff_session IS NULL AND handoff_sequence IS NULL AND handoff_instance IS NULL)
        OR (handoff_session IS NOT NULL AND handoff_session<>session_id
            AND handoff_sequence IS NOT NULL AND handoff_sequence>=0
            AND handoff_instance IS NOT NULL AND handoff_instance>instance AND resumed_at IS NOT NULL));
CREATE UNIQUE INDEX model_handoff_from ON model_pauses(execution_id,session_id) WHERE handoff_session IS NOT NULL;
CREATE UNIQUE INDEX model_handoff_to ON model_pauses(execution_id,handoff_session) WHERE handoff_session IS NOT NULL;

CREATE FUNCTION checkpoint_session_continues(exec_id bigint, through_sequence integer, original text, target text)
RETURNS boolean LANGUAGE sql STABLE STRICT AS $$
    WITH RECURSIVE lineage(session, step) AS (
        SELECT original, 0::bigint
        UNION ALL
        SELECT p.handoff_session, p.resource_step FROM lineage l JOIN model_pauses p
            ON p.execution_id=exec_id AND p.session_id=l.session AND p.resource_step>l.step
            AND p.handoff_sequence<=through_sequence
    )
    SELECT EXISTS(SELECT 1 FROM lineage WHERE session=target);
$$;

CREATE OR REPLACE VIEW candidate_rule_context AS
SELECT e.id AS execution_id, (
    SELECT jsonb_object_agg(k,v) FROM (VALUES
        ('global_rules',to_jsonb(g)), ('decisions',NULLIF(e.decision_context,'[]'::jsonb)),
        ('acceptance_feedback',prior.feedback),
        ('owner_instructions',(
            SELECT jsonb_agg(jsonb_build_object('sequence',processed.sequence,'session_id',processed.session_id,
                'instructions',queued.next_instructions) ORDER BY processed.sequence)
            FROM execution_checkpoints queued JOIN execution_checkpoints processed
                ON processed.execution_id=queued.execution_id AND processed.sequence=queued.sequence+1
            WHERE queued.execution_id=e.id AND queued.next_instructions<>'[]'
                AND checkpoint_session_continues(e.id,processed.sequence,queued.session_id,processed.session_id)
        )),
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
