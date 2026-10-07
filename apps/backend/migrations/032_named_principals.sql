-- Stable individual identities replace role-wide ownership for new work.
-- Historic role-only rows retain explicit legacy principals.
CREATE TABLE app_principals (
 id uuid PRIMARY KEY,
 display_name text NOT NULL CHECK (length(trim(display_name)) BETWEEN 2 AND 120),
 role text NOT NULL CHECK (role IN ('admin','reviewer')),
 created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO app_principals(id,display_name,role) VALUES
 ('00000000-0000-4000-8000-000000000001','Legacy administrator','admin'),
 ('00000000-0000-4000-8000-000000000002','Legacy reviewer','reviewer');

ALTER TABLE answer_jobs ADD COLUMN owner_principal_id uuid REFERENCES app_principals(id);
UPDATE answer_jobs SET owner_principal_id=CASE WHEN owner_role='reviewer' THEN '00000000-0000-4000-8000-000000000002'::uuid ELSE '00000000-0000-4000-8000-000000000001'::uuid END;
ALTER TABLE answer_jobs ALTER COLUMN owner_principal_id SET NOT NULL;
ALTER TABLE answers ADD COLUMN owner_principal_id uuid REFERENCES app_principals(id);
UPDATE answers SET owner_principal_id=CASE WHEN owner_role='reviewer' THEN '00000000-0000-4000-8000-000000000002'::uuid ELSE '00000000-0000-4000-8000-000000000001'::uuid END;
ALTER TABLE answers ALTER COLUMN owner_principal_id SET NOT NULL;

ALTER TABLE activity_event_reads ADD COLUMN actor_principal_id uuid REFERENCES app_principals(id);
UPDATE activity_event_reads SET actor_principal_id=CASE WHEN actor_role='reviewer' THEN '00000000-0000-4000-8000-000000000002'::uuid ELSE '00000000-0000-4000-8000-000000000001'::uuid END;
ALTER TABLE activity_event_reads ALTER COLUMN actor_principal_id SET NOT NULL;
ALTER TABLE activity_event_reads DROP CONSTRAINT activity_event_reads_pkey;
ALTER TABLE activity_event_reads ADD PRIMARY KEY(event_id,actor_principal_id);

ALTER TABLE rights_decisions ADD COLUMN reviewer_principal_id uuid REFERENCES app_principals(id);
ALTER TABLE rights_decisions DISABLE TRIGGER rights_decisions_immutable;
UPDATE rights_decisions SET reviewer_principal_id='00000000-0000-4000-8000-000000000001';
ALTER TABLE rights_decisions ENABLE TRIGGER rights_decisions_immutable;
ALTER TABLE publications ADD COLUMN approved_by_principal_id uuid REFERENCES app_principals(id);
ALTER TABLE publications ADD COLUMN approved_at timestamptz;
ALTER TABLE publications ADD COLUMN published_by_principal_id uuid REFERENCES app_principals(id);
UPDATE publications SET published_by_principal_id='00000000-0000-4000-8000-000000000001' WHERE published_at IS NOT NULL;
ALTER TABLE source_access_decisions ADD COLUMN actor_principal_id uuid REFERENCES app_principals(id);
UPDATE source_access_decisions SET actor_principal_id='00000000-0000-4000-8000-000000000001';

CREATE TABLE page_review_decisions (
 id uuid PRIMARY KEY,
 page_id uuid NOT NULL REFERENCES pages(id),
 processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 actor_principal_id uuid NOT NULL REFERENCES app_principals(id),
 previous_kind text NOT NULL,
 decision_kind text NOT NULL,
 rationale text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX page_review_decisions_page_idx ON page_review_decisions(page_id,created_at DESC);
CREATE TRIGGER page_review_decisions_immutable BEFORE UPDATE OR DELETE ON page_review_decisions FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();

CREATE OR REPLACE FUNCTION guard_publication_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'publication snapshot is immutable'; END IF;
 IF OLD.published_at IS NOT NULL AND
  (OLD.processing_revision_id IS DISTINCT FROM NEW.processing_revision_id OR OLD.edition_id IS DISTINCT FROM NEW.edition_id OR OLD.source_asset_id IS DISTINCT FROM NEW.source_asset_id OR OLD.rights_decision_id IS DISTINCT FROM NEW.rights_decision_id OR OLD.metadata_snapshot IS DISTINCT FROM NEW.metadata_snapshot OR OLD.rights_snapshot IS DISTINCT FROM NEW.rights_snapshot OR OLD.page_labels_snapshot IS DISTINCT FROM NEW.page_labels_snapshot OR OLD.approved_by_principal_id IS DISTINCT FROM NEW.approved_by_principal_id OR OLD.approved_at IS DISTINCT FROM NEW.approved_at OR OLD.published_by_principal_id IS DISTINCT FROM NEW.published_by_principal_id OR OLD.published_at IS DISTINCT FROM NEW.published_at) THEN
  RAISE EXCEPTION 'publication snapshot is immutable';
 END IF;
 RETURN NEW;
END $$;
