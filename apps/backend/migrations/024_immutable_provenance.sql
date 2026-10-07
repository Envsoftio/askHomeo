-- Each imported PDF has a bibliographic snapshot, acquisition record, immutable
-- asset record, and a processing revision. Existing sources are backfilled.
CREATE TABLE editions (
 id uuid PRIMARY KEY, title text NOT NULL, author text NOT NULL,
 edition text NOT NULL, publication_info text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE source_records (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id),
 repository text NOT NULL, source_url text NOT NULL,
 pdf_origin_url text NOT NULL, rights_statement text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE source_assets (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id),
 kind text NOT NULL CHECK (kind='pdf'), pdf_path text NOT NULL,
 sha256 char(64) NOT NULL, byte_size bigint NOT NULL CHECK (byte_size>0),
 origin_url text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE processing_revisions (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id),
 source_asset_id uuid NOT NULL REFERENCES source_assets(id),
 status text NOT NULL CHECK (status IN ('queued','processing','review','published','failed')),
 component_versions_json jsonb NOT NULL DEFAULT '{}'::jsonb,
 configuration_json jsonb NOT NULL DEFAULT '{}'::jsonb,
 created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE(id,source_id,source_asset_id), UNIQUE(id,source_id)
);
ALTER TABLE sources ADD COLUMN edition_id uuid REFERENCES editions(id);
ALTER TABLE sources ADD COLUMN source_record_id uuid REFERENCES source_records(id);
ALTER TABLE sources ADD COLUMN primary_asset_id uuid REFERENCES source_assets(id);
ALTER TABLE sources ADD COLUMN current_revision_id uuid REFERENCES processing_revisions(id);
ALTER TABLE sources ADD COLUMN published_revision_id uuid REFERENCES processing_revisions(id);
ALTER TABLE sources ADD COLUMN supersedes_source_id uuid REFERENCES sources(id);
ALTER TABLE sources ADD COLUMN superseded_at timestamptz;
CREATE INDEX sources_supersedes_idx ON sources(supersedes_source_id);
CREATE INDEX sources_current_idx ON sources(status,superseded_at) WHERE status='published';

INSERT INTO editions(id,title,author,edition,publication_info,created_at)
SELECT s.id,s.title,s.author,s.edition,s.publication_info,s.created_at FROM sources s;
INSERT INTO source_records(id,source_id,repository,source_url,pdf_origin_url,rights_statement,created_at)
SELECT s.id,s.id,s.repository,coalesce(s.source_url,''),s.pdf_origin_url,s.rights_statement,s.created_at FROM sources s;
INSERT INTO source_assets(id,source_id,kind,pdf_path,sha256,byte_size,origin_url,created_at)
SELECT s.id,s.id,'pdf',s.pdf_path,s.pdf_sha256,s.pdf_bytes,s.pdf_origin_url,s.created_at FROM sources s;
INSERT INTO processing_revisions(id,source_id,source_asset_id,status,component_versions_json,configuration_json,created_at,completed_at)
SELECT s.id,s.id,s.id,
 CASE WHEN s.status='published' OR s.status='disabled' THEN 'published' WHEN s.status='approved' OR s.status='review' THEN 'review' WHEN s.status='processing' THEN 'processing' WHEN s.status='failed' THEN 'failed' ELSE 'queued' END,
 jsonb_build_object('ingest','legacy-v1','ocr','tesseract-or-embedded','page_qa','automatic-v1'),
 jsonb_build_object('pdf_sha256',s.pdf_sha256,'page_count',s.page_count),
 s.created_at,CASE WHEN s.status IN ('review','approved','published','disabled') THEN s.updated_at END FROM sources s;
UPDATE sources SET edition_id=id,source_record_id=id,primary_asset_id=id,current_revision_id=id,
 published_revision_id=CASE WHEN status IN ('published','disabled') THEN id END;

ALTER TABLE pages ADD COLUMN processing_revision_id uuid;
ALTER TABLE pages ADD COLUMN source_asset_id uuid;
ALTER TABLE pages DISABLE TRIGGER pages_publication_guard;
UPDATE pages p SET processing_revision_id=s.current_revision_id,source_asset_id=s.primary_asset_id FROM sources s WHERE s.id=p.source_id;
ALTER TABLE pages ENABLE TRIGGER pages_publication_guard;
ALTER TABLE pages ALTER COLUMN processing_revision_id SET NOT NULL;
ALTER TABLE pages ALTER COLUMN source_asset_id SET NOT NULL;
ALTER TABLE pages ADD CONSTRAINT pages_revision_asset_fk FOREIGN KEY(processing_revision_id,source_id,source_asset_id)
 REFERENCES processing_revisions(id,source_id,source_asset_id);
CREATE UNIQUE INDEX pages_id_source_revision_idx ON pages(id,source_id,processing_revision_id);
CREATE UNIQUE INDEX pages_revision_asset_pdf_idx ON pages(processing_revision_id,source_asset_id,pdf_page_index);
ALTER TABLE chunks ADD COLUMN processing_revision_id uuid;
ALTER TABLE chunks DISABLE TRIGGER chunks_publication_guard;
UPDATE chunks c SET processing_revision_id=p.processing_revision_id FROM pages p WHERE p.id=c.page_id;
ALTER TABLE chunks ENABLE TRIGGER chunks_publication_guard;
ALTER TABLE chunks ALTER COLUMN processing_revision_id SET NOT NULL;
ALTER TABLE chunks ADD CONSTRAINT chunks_revision_page_fk FOREIGN KEY(page_id,source_id,processing_revision_id)
 REFERENCES pages(id,source_id,processing_revision_id);
ALTER TABLE jobs ADD COLUMN processing_revision_id uuid;
UPDATE jobs j SET processing_revision_id=s.current_revision_id FROM sources s WHERE s.id=j.source_id;
ALTER TABLE jobs ALTER COLUMN processing_revision_id SET NOT NULL;
ALTER TABLE jobs ADD CONSTRAINT jobs_revision_fk FOREIGN KEY(processing_revision_id,source_id)
 REFERENCES processing_revisions(id,source_id);

ALTER TABLE publications ADD COLUMN processing_revision_id uuid REFERENCES processing_revisions(id);
ALTER TABLE publications ADD COLUMN edition_id uuid REFERENCES editions(id);
ALTER TABLE publications ADD COLUMN source_asset_id uuid REFERENCES source_assets(id);
ALTER TABLE publications ADD COLUMN metadata_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE publications ADD COLUMN rights_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE publications ADD COLUMN page_labels_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE publications ADD COLUMN published_at timestamptz;
UPDATE publications p SET processing_revision_id=s.published_revision_id,edition_id=s.edition_id,
 source_asset_id=s.primary_asset_id,
 metadata_snapshot=jsonb_build_object('title',s.title,'author',s.author,'edition',s.edition,'publication_info',s.publication_info,'repository',s.repository,'source_url',coalesce(s.source_url,'')),
 rights_snapshot=jsonb_build_object('decision',s.rights_status,'statement',s.rights_statement,'note',coalesce(s.review_note,''),'reviewed_at',s.reviewed_at),
 published_at=p.created_at FROM sources s WHERE s.id=p.source_id;
UPDATE publications pub SET page_labels_snapshot=(SELECT coalesce(jsonb_object_agg(p.pdf_page_index,coalesce(p.printed_label,'')),'{}'::jsonb) FROM pages p WHERE p.source_id=pub.source_id);

CREATE OR REPLACE FUNCTION populate_source_provenance() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid uuid := gen_random_uuid(); rid uuid := gen_random_uuid(); aid uuid := gen_random_uuid(); vid uuid := gen_random_uuid();
BEGIN
 INSERT INTO editions(id,title,author,edition,publication_info) VALUES(eid,NEW.title,NEW.author,NEW.edition,NEW.publication_info);
 INSERT INTO source_records(id,source_id,repository,source_url,pdf_origin_url,rights_statement)
 VALUES(rid,NEW.id,NEW.repository,coalesce(NEW.source_url,''),NEW.pdf_origin_url,NEW.rights_statement);
 INSERT INTO source_assets(id,source_id,kind,pdf_path,sha256,byte_size,origin_url)
 VALUES(aid,NEW.id,'pdf',NEW.pdf_path,NEW.pdf_sha256,NEW.pdf_bytes,NEW.pdf_origin_url);
 INSERT INTO processing_revisions(id,source_id,source_asset_id,status,component_versions_json,configuration_json)
 VALUES(vid,NEW.id,aid,'queued',jsonb_build_object('ingest','local-v1','ocr','tesseract-or-embedded','page_qa','automatic-v1'),jsonb_build_object('pdf_sha256',NEW.pdf_sha256,'page_count',NEW.page_count));
 UPDATE sources SET edition_id=eid,source_record_id=rid,primary_asset_id=aid,current_revision_id=vid WHERE id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER sources_provenance_insert AFTER INSERT ON sources FOR EACH ROW EXECUTE FUNCTION populate_source_provenance();

CREATE OR REPLACE FUNCTION refresh_source_metadata_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid uuid := gen_random_uuid(); rid uuid := gen_random_uuid();
BEGIN
 IF OLD.status IN ('published','disabled') THEN RAISE EXCEPTION 'published source metadata is immutable'; END IF;
 INSERT INTO editions(id,title,author,edition,publication_info) VALUES(eid,NEW.title,NEW.author,NEW.edition,NEW.publication_info);
 INSERT INTO source_records(id,source_id,repository,source_url,pdf_origin_url,rights_statement)
 VALUES(rid,NEW.id,NEW.repository,coalesce(NEW.source_url,''),NEW.pdf_origin_url,NEW.rights_statement);
 UPDATE sources SET edition_id=eid,source_record_id=rid WHERE id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER sources_metadata_snapshot AFTER UPDATE OF title,author,edition,publication_info,repository,source_url,rights_statement ON sources
 FOR EACH ROW WHEN (OLD.title IS DISTINCT FROM NEW.title OR OLD.author IS DISTINCT FROM NEW.author OR OLD.edition IS DISTINCT FROM NEW.edition OR OLD.publication_info IS DISTINCT FROM NEW.publication_info OR OLD.repository IS DISTINCT FROM NEW.repository OR OLD.source_url IS DISTINCT FROM NEW.source_url OR OLD.rights_statement IS DISTINCT FROM NEW.rights_statement)
 EXECUTE FUNCTION refresh_source_metadata_snapshot();

CREATE OR REPLACE FUNCTION assign_revision_lineage() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='pages' THEN
  SELECT current_revision_id,primary_asset_id INTO NEW.processing_revision_id,NEW.source_asset_id FROM sources WHERE id=NEW.source_id;
 ELSE
  SELECT current_revision_id INTO NEW.processing_revision_id FROM sources WHERE id=NEW.source_id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER pages_revision_insert BEFORE INSERT ON pages FOR EACH ROW EXECUTE FUNCTION assign_revision_lineage();
CREATE TRIGGER chunks_revision_insert BEFORE INSERT ON chunks FOR EACH ROW EXECUTE FUNCTION assign_revision_lineage();
CREATE TRIGGER jobs_revision_insert BEFORE INSERT ON jobs FOR EACH ROW EXECUTE FUNCTION assign_revision_lineage();

CREATE OR REPLACE FUNCTION guard_immutable_provenance() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'provenance snapshots are immutable';
END $$;
CREATE TRIGGER editions_immutable BEFORE UPDATE OR DELETE ON editions FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();
CREATE TRIGGER source_records_immutable BEFORE UPDATE OR DELETE ON source_records FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();
CREATE TRIGGER source_assets_immutable BEFORE UPDATE OR DELETE ON source_assets FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();
CREATE OR REPLACE FUNCTION guard_revision_configuration() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'processing revision is immutable'; END IF;
 IF OLD.source_id IS DISTINCT FROM NEW.source_id OR OLD.source_asset_id IS DISTINCT FROM NEW.source_asset_id THEN RAISE EXCEPTION 'processing revision identity is immutable'; END IF;
 IF (OLD.component_versions_json IS DISTINCT FROM NEW.component_versions_json OR OLD.configuration_json IS DISTINCT FROM NEW.configuration_json)
    AND (OLD.status NOT IN ('queued','processing') OR OLD.component_versions_json ? 'ghostscript') THEN
  RAISE EXCEPTION 'processing configuration is immutable after execution';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER revision_config_immutable BEFORE UPDATE OF source_id,source_asset_id,component_versions_json,configuration_json OR DELETE ON processing_revisions FOR EACH ROW EXECUTE FUNCTION guard_revision_configuration();

CREATE OR REPLACE FUNCTION guard_publication_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'publication snapshot is immutable'; END IF;
 IF OLD.published_at IS NOT NULL AND
  (OLD.processing_revision_id IS DISTINCT FROM NEW.processing_revision_id OR OLD.edition_id IS DISTINCT FROM NEW.edition_id OR OLD.source_asset_id IS DISTINCT FROM NEW.source_asset_id OR OLD.metadata_snapshot IS DISTINCT FROM NEW.metadata_snapshot OR OLD.rights_snapshot IS DISTINCT FROM NEW.rights_snapshot OR OLD.page_labels_snapshot IS DISTINCT FROM NEW.page_labels_snapshot OR OLD.published_at IS DISTINCT FROM NEW.published_at) THEN
  RAISE EXCEPTION 'publication snapshot is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER publication_snapshot_immutable BEFORE UPDATE OR DELETE ON publications FOR EACH ROW EXECUTE FUNCTION guard_publication_snapshot();

CREATE OR REPLACE FUNCTION guard_source_asset_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.status IN ('published','disabled') AND
  (OLD.pdf_path IS DISTINCT FROM NEW.pdf_path OR OLD.pdf_sha256 IS DISTINCT FROM NEW.pdf_sha256 OR OLD.pdf_bytes IS DISTINCT FROM NEW.pdf_bytes OR OLD.pdf_origin_url IS DISTINCT FROM NEW.pdf_origin_url OR OLD.primary_asset_id IS DISTINCT FROM NEW.primary_asset_id OR OLD.current_revision_id IS DISTINCT FROM NEW.current_revision_id) THEN
  RAISE EXCEPTION 'published source asset and revision are immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER source_asset_immutable BEFORE UPDATE ON sources FOR EACH ROW EXECUTE FUNCTION guard_source_asset_change();
