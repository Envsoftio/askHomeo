CREATE TABLE rights_decisions (
 id uuid PRIMARY KEY,
 source_id uuid NOT NULL REFERENCES sources(id),
 processing_revision_id uuid NOT NULL,
 source_asset_id uuid NOT NULL,
 raw_statement text NOT NULL,
 evidence_url text NOT NULL DEFAULT '',
 intended_use text NOT NULL DEFAULT 'local research',
 jurisdiction text NOT NULL DEFAULT '',
 decision text NOT NULL CHECK (decision IN ('allowed','denied')),
 reviewer_role text NOT NULL,
 rationale text NOT NULL,
 restrictions text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(processing_revision_id,source_id,source_asset_id)
  REFERENCES processing_revisions(id,source_id,source_asset_id)
);
CREATE INDEX rights_decisions_source_idx ON rights_decisions(source_id,created_at DESC);
INSERT INTO rights_decisions(id,source_id,processing_revision_id,source_asset_id,raw_statement,decision,reviewer_role,rationale,created_at)
SELECT s.id,s.id,s.current_revision_id,s.primary_asset_id,s.rights_statement,s.rights_status,'admin (legacy role only)',coalesce(s.review_note,''),s.reviewed_at
FROM sources s WHERE s.reviewed_at IS NOT NULL AND s.rights_status IN ('allowed','denied');
ALTER TABLE sources ADD COLUMN rights_decision_id uuid REFERENCES rights_decisions(id);
UPDATE sources SET rights_decision_id=id WHERE id IN (SELECT id FROM rights_decisions);
ALTER TABLE publications ADD COLUMN rights_decision_id uuid REFERENCES rights_decisions(id);
ALTER TABLE publications ADD COLUMN published_by_role text NOT NULL DEFAULT 'admin (legacy role only)';
UPDATE publications p SET rights_decision_id=s.rights_decision_id FROM sources s WHERE s.id=p.source_id;
CREATE TRIGGER rights_decisions_immutable BEFORE UPDATE OR DELETE ON rights_decisions FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();
CREATE OR REPLACE FUNCTION guard_publication_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'publication snapshot is immutable'; END IF;
 IF OLD.published_at IS NOT NULL AND
  (OLD.processing_revision_id IS DISTINCT FROM NEW.processing_revision_id OR OLD.edition_id IS DISTINCT FROM NEW.edition_id OR OLD.source_asset_id IS DISTINCT FROM NEW.source_asset_id OR OLD.rights_decision_id IS DISTINCT FROM NEW.rights_decision_id OR OLD.metadata_snapshot IS DISTINCT FROM NEW.metadata_snapshot OR OLD.rights_snapshot IS DISTINCT FROM NEW.rights_snapshot OR OLD.page_labels_snapshot IS DISTINCT FROM NEW.page_labels_snapshot OR OLD.published_at IS DISTINCT FROM NEW.published_at) THEN
  RAISE EXCEPTION 'publication snapshot is immutable';
 END IF;
 RETURN NEW;
END $$;
