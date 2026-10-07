CREATE OR REPLACE FUNCTION guard_published_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid;
BEGIN
 sid:=CASE WHEN TG_OP='DELETE' THEN OLD.source_id ELSE NEW.source_id END;
 IF EXISTS(SELECT 1 FROM sources WHERE id=sid AND status IN ('published','disabled')) THEN
  RAISE EXCEPTION 'published evidence is immutable; create a new revision';
 END IF;
 RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;
CREATE TRIGGER pages_publication_guard BEFORE INSERT OR UPDATE OR DELETE ON pages FOR EACH ROW EXECUTE FUNCTION guard_published_evidence();
CREATE TRIGGER chunks_publication_guard BEFORE INSERT OR UPDATE OR DELETE ON chunks FOR EACH ROW EXECUTE FUNCTION guard_published_evidence();
