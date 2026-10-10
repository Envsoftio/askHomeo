-- Permanent deletion is restricted to unused, idle drafts. File cleanup is
-- durable and independent of source rows so storage outages are retryable.
CREATE TABLE deleted_source_files (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), source_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('pdf','html','txt')), sha256 char(64) NOT NULL,
 locator text NOT NULL, retry_after timestamptz NOT NULL DEFAULT now(),
 error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX deleted_source_files_pending ON deleted_source_files(retry_after);

CREATE FUNCTION source_deletion_blocker(sid uuid) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT CASE
 WHEN NOT EXISTS(SELECT 1 FROM sources WHERE id=sid) THEN 'Source not found.'
 WHEN EXISTS(SELECT 1 FROM sources WHERE id=sid AND (status IN ('published','approved') OR published_revision_id IS NOT NULL))
   OR EXISTS(SELECT 1 FROM publications WHERE source_id=sid)
   OR EXISTS(SELECT 1 FROM processing_revisions WHERE source_id=sid AND status='published')
   OR EXISTS(SELECT 1 FROM source_access_decisions WHERE source_id=sid AND previous_status='published')
 THEN 'This source has publication history. Remove it from the library instead.'
 WHEN EXISTS(SELECT 1 FROM sources WHERE supersedes_source_id=sid)
 THEN 'Another source depends on this version. Delete the unused replacement first.'
 WHEN EXISTS(SELECT 1 FROM answer_citations a JOIN chunks c ON c.id=a.chunk_id WHERE c.source_id=sid)
   OR EXISTS(SELECT 1 FROM answer_candidates a JOIN chunks c ON c.id=a.chunk_id WHERE c.source_id=sid)
   OR EXISTS(SELECT 1 FROM answer_claim_support a JOIN chunks c ON c.id=a.chunk_id WHERE c.source_id=sid)
   OR EXISTS(SELECT 1 FROM answer_source_filters WHERE source_id=sid)
   OR EXISTS(SELECT 1 FROM answer_jobs WHERE sid=ANY(source_ids))
 THEN 'Saved research references this source. Remove it from the library instead.'
 WHEN EXISTS(SELECT 1 FROM jobs WHERE source_id=sid AND status='running')
   OR EXISTS(SELECT 1 FROM collection_items i JOIN collection_capture_jobs c ON c.snapshot_id=i.snapshot_id WHERE i.source_id=sid AND c.state='running')
 THEN 'Processing is running. Wait for it to finish before permanent deletion.'
 ELSE '' END
$$;

CREATE FUNCTION purging_draft(sid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT sid::text = current_setting('app.purge_source',true)
 AND EXISTS(SELECT 1 FROM sources WHERE id=sid)
 AND source_deletion_blocker(sid)=''
$$;
CREATE OR REPLACE FUNCTION guard_immutable_provenance() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid; row_data jsonb := to_jsonb(OLD);
BEGIN
 IF TG_OP='DELETE' THEN
  sid := (row_data->>'source_id')::uuid;
  IF sid IS NULL AND row_data ? 'page_id' THEN SELECT source_id INTO sid FROM pages WHERE id=(row_data->>'page_id')::uuid; END IF;
  IF TG_TABLE_NAME='editions' AND row_data->>'id'=current_setting('app.purge_edition',true) THEN
   sid := nullif(current_setting('app.purge_source',true),'')::uuid;
  END IF;
  IF purging_draft(sid) THEN RETURN OLD; END IF;
 END IF;
 RAISE EXCEPTION 'provenance snapshots are immutable';
END $$;
CREATE OR REPLACE FUNCTION guard_revision_configuration() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND purging_draft(OLD.source_id) THEN RETURN OLD; END IF;
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'processing revision is immutable'; END IF;
 IF OLD.source_id IS DISTINCT FROM NEW.source_id OR OLD.source_asset_id IS DISTINCT FROM NEW.source_asset_id THEN RAISE EXCEPTION 'processing revision identity is immutable'; END IF;
 IF (OLD.component_versions_json IS DISTINCT FROM NEW.component_versions_json OR OLD.configuration_json IS DISTINCT FROM NEW.configuration_json)
    AND (OLD.status NOT IN ('queued','processing') OR OLD.component_versions_json ? 'ghostscript') THEN
  RAISE EXCEPTION 'processing configuration is immutable after execution';
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION guard_published_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid;
BEGIN
 IF TG_OP='DELETE' AND purging_draft(OLD.source_id) THEN RETURN OLD; END IF;
 sid:=CASE WHEN TG_OP='DELETE' THEN OLD.source_id ELSE NEW.source_id END;
 IF EXISTS(SELECT 1 FROM sources WHERE id=sid AND status IN ('published','disabled')) THEN
  RAISE EXCEPTION 'published evidence is immutable; create a new revision';
 END IF;
 RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE FUNCTION delete_unused_source(sid uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE blocker text; eid uuid;
BEGIN
 -- Serialize queued job claims with this deletion. Running workers are refused.
 PERFORM 1 FROM jobs WHERE source_id=sid FOR UPDATE;
 SELECT edition_id INTO eid FROM sources WHERE id=sid FOR UPDATE;
 blocker := source_deletion_blocker(sid);
 IF blocker<>'' THEN RAISE EXCEPTION '%',blocker USING ERRCODE='P0001'; END IF;
 PERFORM set_config('app.purge_source',sid::text,true);
 PERFORM set_config('app.purge_edition',eid::text,true);
 INSERT INTO deleted_source_files(source_id,kind,sha256,locator)
 SELECT sid,kind,sha256,coalesce(object_locator,pdf_path) FROM source_assets WHERE source_id=sid;
 DELETE FROM structured_review_decisions WHERE source_id=sid;
 DELETE FROM rubric_cross_references WHERE source_id=sid;
 DELETE FROM rubric_remedy_locations WHERE source_id=sid;
 DELETE FROM rubric_remedies WHERE source_id=sid;
 DELETE FROM structured_entry_locations WHERE source_id=sid;
 DELETE FROM structured_entries WHERE source_id=sid;
 DELETE FROM remedy_aliases WHERE source_id=sid;
 DELETE FROM literature_section_category_decisions WHERE source_id=sid;
 DELETE FROM literature_section_categories WHERE source_id=sid;
 DELETE FROM literature_category_decisions WHERE source_id=sid;
 DELETE FROM literature_category_suggestions WHERE source_id=sid;
 DELETE FROM chunk_embeddings WHERE chunk_id IN(SELECT id FROM chunks WHERE source_id=sid);
 DELETE FROM chunks WHERE source_id=sid;
 DELETE FROM document_locations WHERE source_id=sid;
 DELETE FROM document_block_decisions WHERE source_id=sid;
 DELETE FROM document_blocks WHERE source_id=sid;
 DELETE FROM page_text_repairs WHERE page_id IN(SELECT id FROM pages WHERE source_id=sid);
 DELETE FROM page_review_decisions WHERE page_id IN(SELECT id FROM pages WHERE source_id=sid);
 DELETE FROM pages WHERE source_id=sid;
 DELETE FROM activity_event_reads WHERE event_id IN(SELECT id FROM activity_events WHERE source_job_id IN(SELECT id FROM jobs WHERE source_id=sid));
 DELETE FROM activity_events WHERE source_job_id IN(SELECT id FROM jobs WHERE source_id=sid);
 DELETE FROM jobs WHERE source_id=sid;
 DELETE FROM document_acquisitions WHERE source_id=sid;
 DELETE FROM source_removals WHERE source_id=sid;
 DELETE FROM source_access_decisions WHERE source_id=sid;
 UPDATE collection_items SET source_id=NULL WHERE source_id=sid;
 DELETE FROM doi_references WHERE source_id=sid;
 UPDATE sources SET edition_id=NULL,source_record_id=NULL,primary_asset_id=NULL,current_revision_id=NULL,rights_decision_id=NULL WHERE id=sid;
 DELETE FROM rights_decisions WHERE source_id=sid;
 DELETE FROM processing_revisions WHERE source_id=sid;
 DELETE FROM source_assets WHERE source_id=sid;
 DELETE FROM source_records WHERE source_id=sid;
 DELETE FROM editions WHERE id=eid AND NOT EXISTS(SELECT 1 FROM sources WHERE edition_id=eid) AND NOT EXISTS(SELECT 1 FROM publications WHERE edition_id=eid);
 DELETE FROM sources WHERE id=sid;
 PERFORM set_config('app.purge_source','',true);
 PERFORM set_config('app.purge_edition','',true);
END $$;
CREATE OR REPLACE FUNCTION guard_source_asset_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF purging_draft(OLD.id) THEN RETURN NEW; END IF;
 IF OLD.status IN ('published','disabled') AND
  (OLD.pdf_path IS DISTINCT FROM NEW.pdf_path OR OLD.pdf_sha256 IS DISTINCT FROM NEW.pdf_sha256 OR OLD.pdf_bytes IS DISTINCT FROM NEW.pdf_bytes OR OLD.pdf_origin_url IS DISTINCT FROM NEW.pdf_origin_url OR OLD.primary_asset_id IS DISTINCT FROM NEW.primary_asset_id OR OLD.current_revision_id IS DISTINCT FROM NEW.current_revision_id) THEN
  RAISE EXCEPTION 'published source asset and revision are immutable';
 END IF;
 RETURN NEW;
END $$;
