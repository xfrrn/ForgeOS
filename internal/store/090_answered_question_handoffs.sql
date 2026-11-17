ALTER TABLE checkpoint_resumes
    ADD COLUMN source_instance bigint CHECK (source_instance>0),
    ADD COLUMN handoff_instance bigint,
    ADD COLUMN handoff_session text,
    ADD CHECK ((handoff_session IS NULL AND handoff_instance IS NULL) OR
        (handoff_session IS NOT NULL AND handoff_instance IS NOT NULL AND source_instance IS NOT NULL
         AND handoff_instance>source_instance AND claimed_at IS NOT NULL));

CREATE VIEW writer_handoff_sources AS
SELECT p.execution_id,p.instance AS source_instance,p.session_id AS source_session,
    COALESCE(p.handoff_sequence,c.sequence) AS sequence,p.handoff_instance,p.handoff_session,NULL::bigint AS question_id
FROM model_pauses p JOIN LATERAL (
    SELECT sequence FROM execution_checkpoints c WHERE c.execution_id=p.execution_id AND c.at<=p.requested_at
    ORDER BY sequence DESC LIMIT 1
) c ON true WHERE p.resumed_at IS NOT NULL
UNION ALL
SELECT q.execution_id,r.source_instance,c.session_id,c.sequence+1,r.handoff_instance,r.handoff_session,q.id
FROM checkpoint_resumes r JOIN decision_questions q ON q.id=r.question_id
JOIN execution_checkpoints c ON c.execution_id=q.execution_id AND c.sequence=q.checkpoint_sequence
WHERE r.claimed_at IS NOT NULL AND r.source_instance IS NOT NULL AND q.answer IS NOT NULL
    AND q.withdrawn_at IS NULL AND q.trees IS NOT NULL AND c.session_id IS NOT NULL;

CREATE OR REPLACE FUNCTION checkpoint_session_continues(exec_id bigint, through_sequence integer, original text, target text)
RETURNS boolean LANGUAGE sql STABLE STRICT AS $$
    WITH RECURSIVE lineage(session, instance) AS (
        SELECT original, 0::bigint
        UNION ALL
        SELECT h.handoff_session,h.handoff_instance FROM lineage l JOIN writer_handoff_sources h
            ON h.execution_id=exec_id AND h.source_session=l.session AND h.source_instance>=l.instance
            AND h.handoff_session IS NOT NULL AND h.sequence<=through_sequence
    )
    SELECT EXISTS(SELECT 1 FROM lineage WHERE session=target);
$$;
