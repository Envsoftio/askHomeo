-- Figure pixels are separate reviewed assets. The source XML remains the
-- immutable article original; uploaded images are never indexed as text.
CREATE TABLE document_figure_images (
 id uuid PRIMARY KEY,
 source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
 source_asset_id uuid NOT NULL REFERENCES source_assets(id) ON DELETE CASCADE,
 section_key text NOT NULL,
 sha256 char(64) NOT NULL,
 media_type text NOT NULL CHECK (media_type IN ('image/png','image/jpeg')),
 byte_size bigint NOT NULL CHECK (byte_size>0 AND byte_size<=5242880),
 origin_note text NOT NULL,
 uploaded_by_principal_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(source_asset_id,section_key)
);

ALTER TABLE deleted_source_files DROP CONSTRAINT deleted_source_files_kind_check;
ALTER TABLE deleted_source_files ADD CONSTRAINT deleted_source_files_kind_check CHECK(kind IN ('pdf','html','txt','xml','jats','figure_png','figure_jpg'));

CREATE FUNCTION queue_deleted_figure_image() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO deleted_source_files(source_id,kind,sha256,locator)
 VALUES(OLD.source_id,CASE WHEN OLD.media_type='image/png' THEN 'figure_png' ELSE 'figure_jpg' END,
 OLD.sha256,'data/runtime/assets/figures/'||OLD.sha256||CASE WHEN OLD.media_type='image/png' THEN '.png' ELSE '.jpg' END);
 RETURN OLD;
END $$;
CREATE TRIGGER document_figure_image_cleanup BEFORE DELETE ON document_figure_images
 FOR EACH ROW EXECUTE FUNCTION queue_deleted_figure_image();
