ALTER TABLE agent_skills ADD COLUMN package_key text, ADD COLUMN file_count integer;
-- Retire the sample choices from active settings while keeping past runs and revisions intact.
WITH latest AS (
    SELECT DISTINCT ON (scope_id,purpose) scope_id,purpose,settings FROM agent_config_revisions
    ORDER BY scope_id,purpose,revision DESC
)
INSERT INTO agent_config_revisions(scope_id,purpose,settings)
SELECT scope_id,purpose,jsonb_set(settings,'{skills}',COALESCE((
    SELECT jsonb_agg(i.id ORDER BY i.ord) FROM jsonb_array_elements(settings->'skills') WITH ORDINALITY i(id,ord)
    JOIN agent_skills s ON to_jsonb(s.id)=i.id WHERE NOT s.builtin
),'[]'::jsonb))
FROM latest WHERE EXISTS(SELECT 1 FROM jsonb_array_elements(settings->'skills') i(id)
    JOIN agent_skills s ON to_jsonb(s.id)=i.id WHERE s.builtin);
