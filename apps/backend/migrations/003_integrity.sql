CREATE UNIQUE INDEX IF NOT EXISTS pages_id_source_idx ON pages(id,source_id);
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chunks_page_source_fk') THEN
  ALTER TABLE chunks ADD CONSTRAINT chunks_page_source_fk FOREIGN KEY(page_id,source_id) REFERENCES pages(id,source_id);
 END IF;
END $$;
CREATE OR REPLACE FUNCTION check_chunk_exact() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE page_text text;
BEGIN
 SELECT text_raw INTO page_text FROM pages WHERE id=NEW.page_id;
 IF page_text IS NULL OR substring(page_text from NEW.start_character+1 for NEW.end_character-NEW.start_character) <> NEW.text_exact THEN
  RAISE EXCEPTION 'chunk text or Unicode offsets do not match page OCR';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS chunks_exact_guard ON chunks;
CREATE TRIGGER chunks_exact_guard BEFORE INSERT OR UPDATE OF text_exact,start_character,end_character,page_id ON chunks FOR EACH ROW EXECUTE FUNCTION check_chunk_exact();
