-- sources.pdf_path/pdf_sha256/pdf_bytes/page_count and pages.pdf_page_index are PDF-only.
-- Preserve them and existing citation fields. Non-PDF originals use an immutable
-- source asset and a revision-scoped location without inventing a PDF page.
ALTER TABLE source_assets DROP CONSTRAINT source_assets_kind_check;
ALTER TABLE source_assets ADD CONSTRAINT source_assets_kind_check CHECK (kind IN ('pdf','html','txt','xml'));
ALTER TABLE source_assets ALTER COLUMN pdf_path DROP NOT NULL;
ALTER TABLE source_assets ADD COLUMN object_locator text;
ALTER TABLE source_assets ADD CONSTRAINT source_assets_location_check CHECK (
 (kind='pdf' AND pdf_path IS NOT NULL AND object_locator IS NULL) OR
 (kind IN ('html','txt','xml') AND pdf_path IS NULL AND object_locator IS NOT NULL)
);
CREATE UNIQUE INDEX pages_document_location_identity_idx ON pages(id,source_id,processing_revision_id,source_asset_id);
CREATE TABLE document_locations (
 id uuid PRIMARY KEY,
 source_id uuid NOT NULL REFERENCES sources(id),
 source_asset_id uuid NOT NULL REFERENCES source_assets(id),
 processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 kind text NOT NULL CHECK (kind IN ('pdf_page','section','text_span')),
 pdf_page_id uuid,
 section_key text,
 start_character int,
 end_character int,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (processing_revision_id,source_id,source_asset_id)
  REFERENCES processing_revisions(id,source_id,source_asset_id),
 FOREIGN KEY (pdf_page_id,source_id,processing_revision_id,source_asset_id)
  REFERENCES pages(id,source_id,processing_revision_id,source_asset_id),
 CHECK ((kind='pdf_page' AND pdf_page_id IS NOT NULL AND section_key IS NULL AND start_character IS NULL AND end_character IS NULL)
  OR (kind='section' AND pdf_page_id IS NULL AND section_key IS NOT NULL AND section_key<>'' AND start_character IS NULL AND end_character IS NULL)
  OR (kind='text_span' AND pdf_page_id IS NULL AND section_key IS NOT NULL AND section_key<>'' AND start_character>=0 AND end_character>start_character))
);
CREATE INDEX document_locations_revision_idx ON document_locations(processing_revision_id,source_asset_id);
CREATE TRIGGER document_locations_immutable BEFORE UPDATE OR DELETE ON document_locations
 FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();
