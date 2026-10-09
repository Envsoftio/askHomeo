-- Non-PDF sources have an immutable original and revision, but no PDF page.
ALTER TABLE sources ALTER COLUMN pdf_path DROP NOT NULL;
ALTER TABLE sources ALTER COLUMN pdf_sha256 DROP NOT NULL;
ALTER TABLE sources ALTER COLUMN pdf_bytes DROP NOT NULL;
ALTER TABLE sources ALTER COLUMN page_count DROP NOT NULL;
ALTER TABLE sources ADD COLUMN document_format text NOT NULL DEFAULT 'pdf' CHECK (document_format IN ('pdf','html','txt'));
ALTER TABLE sources ADD COLUMN document_object_locator text;
ALTER TABLE sources ADD COLUMN document_sha256 char(64);
ALTER TABLE sources ADD COLUMN document_bytes bigint;
ALTER TABLE sources ADD COLUMN document_content_type text NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN document_charset_override text NOT NULL DEFAULT '';
ALTER TABLE sources ADD CONSTRAINT sources_format_storage_check CHECK (
 (document_format='pdf' AND pdf_path IS NOT NULL AND pdf_sha256 IS NOT NULL AND pdf_bytes IS NOT NULL AND page_count IS NOT NULL)
 OR (document_format IN ('html','txt') AND pdf_path IS NULL AND pdf_sha256 IS NULL AND pdf_bytes IS NULL AND page_count IS NULL
 AND document_object_locator IS NOT NULL AND document_sha256 IS NOT NULL AND document_bytes>0));

CREATE TABLE document_acquisitions (
 source_id uuid PRIMARY KEY REFERENCES sources(id), source_asset_id uuid NOT NULL REFERENCES source_assets(id),
 requested_url text NOT NULL DEFAULT '', final_url text NOT NULL DEFAULT '',
 transport text NOT NULL CHECK (transport IN ('upload','http','https')),
 content_type text NOT NULL DEFAULT '', acquired_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER document_acquisitions_immutable BEFORE UPDATE OR DELETE ON document_acquisitions
 FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();

CREATE TABLE document_blocks (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id),
 source_asset_id uuid NOT NULL REFERENCES source_assets(id),
 processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 block_index int NOT NULL, section_key text NOT NULL, kind text NOT NULL,
 heading text NOT NULL DEFAULT '', original_text text NOT NULL, reviewed_text text NOT NULL,
 start_byte int NOT NULL, end_byte int NOT NULL,
 review_status text NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','accepted','corrected','excluded')),
 review_note text NOT NULL DEFAULT '', warnings text[] NOT NULL DEFAULT '{}',
 FOREIGN KEY(processing_revision_id,source_id,source_asset_id)
 REFERENCES processing_revisions(id,source_id,source_asset_id),
 UNIQUE(processing_revision_id,block_index),
 CHECK (start_byte>=-1 AND end_byte>=-1)
);
CREATE INDEX document_blocks_source_revision_idx ON document_blocks(source_id,processing_revision_id);
CREATE TABLE document_block_decisions (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id), block_id uuid NOT NULL REFERENCES document_blocks(id),
 processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 actor_principal_id uuid, decision text NOT NULL, rationale text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX document_locations_section_unique_idx ON document_locations(processing_revision_id,section_key)
 WHERE kind='section';
CREATE UNIQUE INDEX document_locations_span_unique_idx ON document_locations(processing_revision_id,section_key,start_character,end_character)
 WHERE kind='text_span';

ALTER TABLE chunks ALTER COLUMN page_id DROP NOT NULL;
ALTER TABLE chunks ADD COLUMN document_block_id uuid REFERENCES document_blocks(id);
CREATE UNIQUE INDEX document_blocks_identity_idx ON document_blocks(id,source_id,processing_revision_id);
ALTER TABLE chunks ADD CONSTRAINT chunks_document_block_fk FOREIGN KEY(document_block_id,source_id,processing_revision_id)
 REFERENCES document_blocks(id,source_id,processing_revision_id);
ALTER TABLE chunks ADD CONSTRAINT chunks_one_origin_check CHECK ((page_id IS NULL) <> (document_block_id IS NULL));
CREATE UNIQUE INDEX chunks_document_block_index_idx ON chunks(document_block_id,chunk_index) WHERE document_block_id IS NOT NULL;

-- One read model lets lexical, vector and answer checks share the same gates.
CREATE VIEW evidence_locations AS
 SELECT c.id AS chunk_id,p.id,p.source_id,p.source_asset_id,p.processing_revision_id,
 p.printed_label,p.scan_page_index,p.pdf_page_index,p.image_url,p.text_raw,p.text_sha256,
 p.page_kind,p.text_qa_status,''::text AS section_key,'pdf'::text AS format
 FROM chunks c JOIN pages p ON p.id=c.page_id
 UNION ALL
 SELECT c.id,b.id,b.source_id,b.source_asset_id,b.processing_revision_id,
 b.section_key,NULL::int,NULL::int,''::text,b.reviewed_text,NULL::char(64),
 'text'::text,'accepted'::text,b.section_key,s.document_format
 FROM chunks c JOIN document_blocks b ON b.id=c.document_block_id JOIN sources s ON s.id=b.source_id
 WHERE b.review_status IN ('accepted','corrected');

CREATE OR REPLACE FUNCTION check_chunk_exact() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source_text text;
BEGIN
 IF NEW.page_id IS NOT NULL THEN
  SELECT text_raw INTO source_text FROM pages WHERE id=NEW.page_id;
 ELSE
  SELECT reviewed_text INTO source_text FROM document_blocks WHERE id=NEW.document_block_id AND review_status IN ('accepted','corrected');
 END IF;
 IF source_text IS NULL OR substring(source_text from NEW.start_character+1 for NEW.end_character-NEW.start_character) <> NEW.text_exact THEN
  RAISE EXCEPTION 'chunk text or Unicode offsets do not match source text';
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION populate_source_provenance() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid uuid := gen_random_uuid(); rid uuid := gen_random_uuid(); aid uuid := gen_random_uuid(); vid uuid := gen_random_uuid();
BEGIN
 INSERT INTO editions(id,title,author,edition,publication_info) VALUES(eid,NEW.title,NEW.author,NEW.edition,NEW.publication_info);
 INSERT INTO source_records(id,source_id,repository,source_url,pdf_origin_url,rights_statement)
 VALUES(rid,NEW.id,NEW.repository,coalesce(NEW.source_url,''),NEW.pdf_origin_url,NEW.rights_statement);
 IF NEW.document_format='pdf' THEN
  INSERT INTO source_assets(id,source_id,kind,pdf_path,sha256,byte_size,origin_url)
  VALUES(aid,NEW.id,'pdf',NEW.pdf_path,NEW.pdf_sha256,NEW.pdf_bytes,NEW.pdf_origin_url);
 ELSE
  INSERT INTO source_assets(id,source_id,kind,object_locator,sha256,byte_size,origin_url)
  VALUES(aid,NEW.id,NEW.document_format,NEW.document_object_locator,NEW.document_sha256,NEW.document_bytes,coalesce(NEW.source_url,''));
 END IF;
 INSERT INTO processing_revisions(id,source_id,source_asset_id,status,component_versions_json,configuration_json)
 VALUES(vid,NEW.id,aid,'queued',jsonb_build_object('ingest','document-v1'),jsonb_build_object('format',NEW.document_format));
 UPDATE sources SET edition_id=eid,source_record_id=rid,primary_asset_id=aid,current_revision_id=vid WHERE id=NEW.id;
 RETURN NEW;
END $$;
