CREATE TABLE experience_entries (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    current_revision integer,
    created_at timestamptz NOT NULL DEFAULT now(),
    withdrawn_at timestamptz,
    UNIQUE(project_id,id)
);

CREATE TABLE experience_changes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id bigint NOT NULL,
    entry_id bigint NOT NULL,
    request_key text NOT NULL,
    expected_revision integer NOT NULL CHECK(expected_revision >= 0),
    new_entry boolean NOT NULL,
    title text NOT NULL,
    source text NOT NULL,
    content text NOT NULL,
    validity text NOT NULL,
    reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    discarded_at timestamptz,
    UNIQUE(project_id,request_key),
    UNIQUE(project_id,id),
    FOREIGN KEY(project_id,entry_id) REFERENCES experience_entries(project_id,id)
);
CREATE INDEX changes_by_entry ON experience_changes(entry_id,id DESC);

ALTER TABLE material_curations
    DROP CONSTRAINT material_curations_check,
    ADD COLUMN experience_change_id bigint UNIQUE,
    ADD FOREIGN KEY(project_id,experience_change_id) REFERENCES experience_changes(project_id,id),
    ADD CHECK (num_nonnulls(key,decision_id,event_id,document_id,experience_change_id)=1 AND (key IS NULL)=(revision IS NULL));

-- A private change participates in the normal extraction/review queue. Keeping
-- its source here also keeps an unadopted draft out of project materials search.
ALTER VIEW curation_sources RENAME TO original_curation_sources;
CREATE VIEW curation_sources AS
SELECT * FROM original_curation_sources
UNION ALL
SELECT c.id,d.project_id,NULL::text,d.id,'memory',d.title,d.source,body.text,
       encode(sha256(convert_to(body.text,'UTF8')),'hex'),d.created_at,NULL::jsonb,
       format('/projects/%s/knowledge/experience/%s/changes/%s/curation',d.project_id,d.entry_id,d.id),
       d.discarded_at IS NULL,NULL::jsonb,NULL::jsonb,NULL::jsonb
FROM experience_changes d JOIN material_curations c ON c.experience_change_id=d.id
CROSS JOIN LATERAL (SELECT d.content || CASE WHEN d.validity='' THEN '' ELSE E'\n\n适用场景：' || d.validity END AS text) body;

CREATE TABLE experience_revisions (
    project_id bigint NOT NULL,
    entry_id bigint NOT NULL,
    revision integer NOT NULL CHECK(revision>0),
    based_on_revision integer NOT NULL CHECK(based_on_revision=revision-1),
    curation_id bigint NOT NULL,
    fact_number integer NOT NULL CHECK(fact_number BETWEEN 1 AND 8),
    change_reason text NOT NULL,
    actor text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(project_id,entry_id,revision),
    UNIQUE(curation_id,fact_number),
    FOREIGN KEY(project_id,entry_id) REFERENCES experience_entries(project_id,id),
    FOREIGN KEY(project_id,curation_id) REFERENCES material_curations(project_id,id)
);
ALTER TABLE experience_entries ADD FOREIGN KEY(project_id,id,current_revision)
    REFERENCES experience_revisions(project_id,entry_id,revision) DEFERRABLE INITIALLY DEFERRED;

CREATE FUNCTION register_reviewed_experience(c material_curations) RETURNS void LANGUAGE plpgsql AS $$
DECLARE number integer; entry bigint;
BEGIN
    IF c.experience_change_id IS NOT NULL OR c.review->>'verdict' IS DISTINCT FROM 'approve'
       OR c.review->>'level' IS DISTINCT FROM 'D1' THEN RETURN; END IF;
    FOR number IN 1..coalesce(jsonb_array_length(c.extraction->'facts'),0) LOOP
        IF NOT EXISTS(SELECT 1 FROM experience_revisions WHERE curation_id=c.id AND fact_number=number) THEN
            INSERT INTO experience_entries(project_id,current_revision,created_at)
                VALUES(c.project_id,1,coalesce(c.reviewed_at,now())) RETURNING id INTO entry;
            INSERT INTO experience_revisions(project_id,entry_id,revision,based_on_revision,curation_id,fact_number,change_reason,actor,created_at)
                VALUES(c.project_id,entry,1,0,c.id,number,'','management',coalesce(c.reviewed_at,now()));
        END IF;
    END LOOP;
END;
$$;
SELECT register_reviewed_experience(c) FROM material_curations c WHERE c.review->>'verdict'='approve' AND c.review->>'level'='D1' ORDER BY c.id;
CREATE FUNCTION register_curation_facts() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM register_reviewed_experience(NEW);
    RETURN NEW;
END;
$$;
CREATE TRIGGER reviewed_experience_facts AFTER INSERT OR UPDATE OF review,extraction ON material_curations
    FOR EACH ROW WHEN (NEW.review IS NOT NULL) EXECUTE FUNCTION register_curation_facts();

CREATE VIEW experience_revision_facts AS
SELECT r.*,s.title,s.source,s.receipt_path,s.content_sha256,s.is_current,s.kind,s.revision AS source_revision,
       c.key,c.decision_id,c.event_id,c.reviewed_at,f.fact->>'statement' AS statement,f.fact->>'quote' AS quote,coalesce(d.validity,'') AS validity,
       strpos(s.content,f.fact->>'quote')-1 AS start_char,
       (e.withdrawn_at IS NULL AND e.current_revision=r.revision AND s.is_current
         AND c.review->>'verdict'='approve' AND c.review->>'level'='D1') AS is_effective
FROM experience_revisions r JOIN experience_entries e ON e.id=r.entry_id
JOIN material_curations c ON c.id=r.curation_id JOIN curation_sources s ON s.curation_id=c.id
LEFT JOIN experience_changes d ON d.id=c.experience_change_id
CROSS JOIN LATERAL (SELECT c.extraction->'facts'->(r.fact_number-1) AS fact) f;

-- Preserve the existing grouped browsing contract and original fact numbers
-- even when one sibling in an extraction is revised or withdrawn.
CREATE OR REPLACE VIEW current_project_experience AS
SELECT c.id,c.project_id,c.key,c.decision_id,c.event_id,s.revision,s.title,s.source,s.kind,s.receipt_path,
       s.content_sha256,s.content,active.facts,c.reviewed_at
FROM material_curations c JOIN curation_sources s ON s.curation_id=c.id
JOIN LATERAL (
    SELECT jsonb_agg((c.extraction->'facts'->(r.fact_number-1)) || jsonb_build_object('fact_number',r.fact_number)
                     ORDER BY r.fact_number) AS facts
    FROM experience_revisions r JOIN experience_entries e ON e.id=r.entry_id
    WHERE r.curation_id=c.id AND r.revision=e.current_revision AND e.withdrawn_at IS NULL
) active ON active.facts IS NOT NULL
WHERE s.is_current AND c.review->>'verdict'='approve' AND c.review->>'level'='D1';

CREATE VIEW experience_change_states AS
SELECT d.*,c.id AS curation_id,c.review,c.reviewed_at,coalesce(c.extraction->'facts','[]') AS facts,
       format('/projects/%s/knowledge/experience/%s/changes/%s/curation',d.project_id,d.entry_id,d.id) AS receipt_path,
       CASE WHEN r.revision IS NOT NULL THEN 'applied'
            WHEN d.discarded_at IS NOT NULL THEN 'discarded'
            WHEN e.withdrawn_at IS NOT NULL OR coalesce(e.current_revision,0)<>d.expected_revision THEN 'obsolete'
            WHEN c.review->>'verdict'='approve' AND c.review->>'level'='D1' THEN 'ready'
            WHEN c.review IS NOT NULL OR c.extraction->'facts'='[]'::jsonb THEN 'rejected'
            ELSE 'pending' END AS status
FROM experience_changes d JOIN experience_entries e ON e.id=d.entry_id
JOIN material_curations c ON c.experience_change_id=d.id
LEFT JOIN experience_revisions r ON r.curation_id=c.id;

CREATE VIEW experience_library AS
SELECT e.id,e.project_id,coalesce(e.current_revision,0) AS current_revision,e.withdrawn_at,
       CASE WHEN e.withdrawn_at IS NOT NULL THEN 'withdrawn'
            WHEN f.is_effective THEN 'current' WHEN f.revision IS NOT NULL THEN 'stale'
            WHEN pending.exists THEN 'pending' ELSE 'rejected' END AS status,
       coalesce(f.title,d.title) AS title,f.statement,f.quote,coalesce(f.source,d.source) AS source,
       coalesce(f.validity,d.validity,'') AS validity,f.reviewed_at,f.receipt_path,pending.exists AS has_pending,
       greatest(e.created_at,e.withdrawn_at,f.created_at,d.created_at,d.reviewed_at,d.discarded_at) AS updated_at
FROM experience_entries e LEFT JOIN experience_revision_facts f
  ON (f.project_id,f.entry_id,f.revision)=(e.project_id,e.id,e.current_revision)
LEFT JOIN LATERAL (SELECT * FROM experience_change_states WHERE entry_id=e.id ORDER BY id DESC LIMIT 1) d ON true
CROSS JOIN LATERAL (SELECT EXISTS(SELECT 1 FROM experience_change_states WHERE entry_id=e.id AND status IN('pending','ready'))) pending;
