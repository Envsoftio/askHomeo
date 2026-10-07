ALTER TABLE doi_references ADD COLUMN retraction_notice_url text NOT NULL DEFAULT '';

-- This article was used by the first live import check before its retraction was noticed.
UPDATE doi_references SET retraction_notice_url='https://doi.org/10.1371/journal.pone.0232415',updated_at=now()
WHERE doi='10.1371/journal.pone.0118440';

UPDATE sources s SET status='disabled',updated_at=now()
FROM doi_references d
WHERE d.source_id=s.id AND d.retraction_notice_url<>'' AND s.status='published';

CREATE OR REPLACE FUNCTION prevent_retracted_source_publication() RETURNS trigger AS $$
BEGIN
 IF NEW.status='published' AND EXISTS (
  SELECT 1 FROM doi_references d WHERE d.source_id=NEW.id AND d.retraction_notice_url<>''
 ) THEN
  RAISE EXCEPTION 'retracted DOI source cannot be published';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER sources_retraction_guard BEFORE INSERT OR UPDATE OF status ON sources
FOR EACH ROW EXECUTE FUNCTION prevent_retracted_source_publication();
