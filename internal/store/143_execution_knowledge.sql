ALTER TABLE executions ADD COLUMN knowledge_tools boolean NOT NULL DEFAULT false;

CREATE TABLE execution_knowledge_reads (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    execution_id bigint NOT NULL REFERENCES executions(id),
    kind text NOT NULL,
    ref_hash bytea NOT NULL,
    evidence jsonb NOT NULL,
    cited_at timestamptz,
    checkpoint_sequence integer,
    UNIQUE (execution_id, kind, ref_hash)
);

CREATE FUNCTION cited_knowledge(run_id bigint, through_sequence integer DEFAULT NULL) RETURNS jsonb
LANGUAGE sql STABLE AS $$
    SELECT jsonb_agg((evidence - 'first_ref' - 'next_ref') || jsonb_build_object('citation_id',id,'cited_at',cited_at)
                     ORDER BY cited_at,id)
    FROM execution_knowledge_reads
    WHERE execution_id=run_id AND cited_at IS NOT NULL
      AND (through_sequence IS NULL OR checkpoint_sequence<=through_sequence)
$$;

CREATE OR REPLACE FUNCTION execution_rule_context(run_id bigint, through_sequence integer) RETURNS jsonb
LANGUAGE sql STABLE AS $$
SELECT (
    SELECT jsonb_object_agg(k,v) FROM (VALUES
        ('global_rules',to_jsonb(g)), ('decisions',NULLIF(e.decision_context,'[]'::jsonb)),
        ('acceptance_feedback',prior.feedback),
        ('knowledge_references',cited_knowledge(e.id,through_sequence)),
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

CREATE OR REPLACE FUNCTION question_context_sources(context jsonb) RETURNS jsonb
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
        UNION ALL
        SELECT 'knowledge:'||(r->>'citation_id'),r->>'content'
        FROM jsonb_array_elements(NULLIF(context->'knowledge_references','null'::jsonb)) r
    ) source WHERE id IS NOT NULL AND text<>'';
$$;
