ALTER TABLE question_advice_claims ADD COLUMN rule_context jsonb;

CREATE FUNCTION execution_rule_context(run_id bigint, through_sequence integer) RETURNS jsonb
LANGUAGE sql STABLE AS $$
SELECT (
    SELECT jsonb_object_agg(k,v) FROM (VALUES
        ('global_rules',to_jsonb(g)), ('decisions',NULLIF(e.decision_context,'[]'::jsonb)),
        ('acceptance_feedback',prior.feedback),
        ('owner_instructions',(
            SELECT jsonb_agg(jsonb_build_object('sequence',processed.sequence,'session_id',processed.session_id,
                'instructions',queued.next_instructions) ORDER BY processed.sequence)
            FROM execution_checkpoints queued JOIN execution_checkpoints processed
                ON processed.execution_id=queued.execution_id AND processed.sequence=queued.sequence+1
            WHERE queued.execution_id=e.id AND (through_sequence IS NULL OR processed.sequence<=through_sequence) AND queued.next_instructions<>'[]'
                AND checkpoint_session_continues(e.id,processed.sequence,queued.session_id,processed.session_id)
        )),
        ('management_references',(
            SELECT jsonb_agg(jsonb_build_object('question_id',q.id,'sequence',q.checkpoint_sequence,
                'decision',q.reference_snapshot,'session_id',q.management_advice->>'session_id')
                ORDER BY q.checkpoint_sequence)
            FROM decision_questions q WHERE q.execution_id=e.id AND q.answered_by='management'
                AND q.reference_snapshot IS NOT NULL AND (through_sequence IS NULL OR q.checkpoint_sequence<=through_sequence)
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
WHERE e.id=run_id;
$$;

CREATE OR REPLACE VIEW candidate_rule_context AS
SELECT e.id AS execution_id, execution_rule_context(e.id,NULL) AS context FROM executions e
WHERE COALESCE(NULLIF(e.ci_candidate_session,''),NULLIF(e.result->>'session_id','')) IS NOT NULL;

CREATE FUNCTION question_rule_context(question_id bigint) RETURNS jsonb
LANGUAGE sql STABLE AS $$
    SELECT COALESCE(execution_rule_context(e.id,current.checkpoint_sequence),'{}'::jsonb)
        || CASE WHEN e.id=current.execution_id THEN '{}'::jsonb ELSE
            jsonb_build_object('implementation_execution_id',e.id,'implementation_commit',e.commit_sha) END
        || jsonb_build_object('owner_clarifications',COALESCE(inherited_owner_answers(e.id),'[]'::jsonb) || COALESCE((
            SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object('id',q.id,'execution_id',q.execution_id,
                'sequence',q.checkpoint_sequence,'question',q.question,'answer',q.answer,'commit_sha',q.commit_sha)) ORDER BY q.id)
            FROM decision_questions q WHERE q.task_id=current.task_id AND q.id<current.id
                AND q.execution_id IN (e.id,current.execution_id) AND q.answer IS NOT NULL AND q.answered_by='owner'
                AND (current.checkpoint_sequence IS NULL OR q.checkpoint_sequence<=current.checkpoint_sequence)
        ),'[]'::jsonb))
    FROM decision_questions current JOIN execution_candidate_identity e ON e.id=COALESCE(
        CASE WHEN current.checkpoint_sequence IS NULL THEN candidate_implementation_execution(current.execution_id) END,
        current.execution_id)
    WHERE current.id=question_id;
$$;

CREATE FUNCTION question_context_sources(context jsonb) RETURNS jsonb
LANGUAGE sql IMMUTABLE AS $$
    SELECT COALESCE(jsonb_agg(jsonb_build_object('id',id,'text',text) ORDER BY id),'[]'::jsonb)
    FROM (
        SELECT 'global-rule:'||(context->'global_rules'->>'revision') AS id,context->'global_rules'->>'content' AS text
        UNION ALL
        SELECT 'acceptance-feedback:'||(context->'acceptance_feedback'->>'execution_id'),context->'acceptance_feedback'->>'note'
        UNION ALL
        SELECT 'owner-answer:'||(a->>'id'),a->>'answer'
        FROM jsonb_array_elements(NULLIF(context->'owner_clarifications','null'::jsonb)) a
        UNION ALL
        SELECT 'owner-command:'||(batch->>'sequence')||':'||(instruction->>'key'),instruction->>'body'
        FROM jsonb_array_elements(NULLIF(context->'owner_instructions','null'::jsonb)) batch
        CROSS JOIN LATERAL jsonb_array_elements(batch->'instructions') instruction
        UNION ALL
        SELECT 'management-decision:'||(r->>'question_id'),r->'decision'->>'text'
        FROM jsonb_array_elements(NULLIF(context->'management_references','null'::jsonb)) r
    ) source WHERE id IS NOT NULL AND text<>'';
$$;
