CREATE OR REPLACE FUNCTION cited_knowledge(run_id bigint, through_sequence integer DEFAULT NULL) RETURNS jsonb
LANGUAGE sql STABLE AS $$
    SELECT jsonb_agg((r.evidence - 'first_ref' - 'next_ref')
                     || jsonb_build_object('citation_id',r.id,'cited_at',r.cited_at)
                     || CASE WHEN k.id IS NULL THEN '{}'::jsonb ELSE
                         jsonb_build_object('publication_status',CASE
                             WHEN k.revoked_at IS NOT NULL THEN 'revoked'
                             WHEN k.superseded_at IS NOT NULL THEN 'superseded'
                             ELSE 'current' END) END
                     ORDER BY r.cited_at,r.id)
    FROM execution_knowledge_reads r
    LEFT JOIN knowledge_shares k ON k.id = CASE WHEN r.kind='shared_experience'
        THEN split_part(r.evidence->>'ref',':',1)::bigint END
    WHERE r.execution_id=run_id AND r.cited_at IS NOT NULL
      AND (through_sequence IS NULL OR r.checkpoint_sequence<=through_sequence)
$$;
