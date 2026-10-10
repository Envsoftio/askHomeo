-- Draft text/QA corrections invalidate rubric context and memberships just as
-- migration 050 does for materia medica. Published evidence stays immutable.
CREATE FUNCTION invalidate_repertory_text_links() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='pages' THEN
  IF NEW.text_raw IS NOT DISTINCT FROM OLD.text_raw AND NEW.page_kind IS NOT DISTINCT FROM OLD.page_kind AND NEW.text_qa_status IS NOT DISTINCT FROM OLD.text_qa_status THEN RETURN NEW; END IF;
  UPDATE structured_entries SET review_status='rejected',validation_note='Source page changed; review the rubric again'
   WHERE kind='repertory_rubric' AND review_status IN ('pending','accepted','corrected') AND id IN
   (SELECT entry_id FROM structured_entry_locations WHERE page_id=OLD.id UNION SELECT rubric_id FROM rubric_remedies rr JOIN rubric_remedy_locations l ON l.association_id=rr.id WHERE l.page_id=OLD.id);
 ELSE
  IF NEW.reviewed_text IS NOT DISTINCT FROM OLD.reviewed_text AND NEW.review_status IS NOT DISTINCT FROM OLD.review_status THEN RETURN NEW; END IF;
  UPDATE structured_entries SET review_status='rejected',validation_note='Source block changed; review the rubric again'
   WHERE kind='repertory_rubric' AND review_status IN ('pending','accepted','corrected') AND id IN
   (SELECT entry_id FROM structured_entry_locations WHERE document_block_id=OLD.id UNION SELECT rubric_id FROM rubric_remedies rr JOIN rubric_remedy_locations l ON l.association_id=rr.id WHERE l.document_block_id=OLD.id);
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER repertory_page_changed BEFORE UPDATE OF text_raw,page_kind,text_qa_status ON pages FOR EACH ROW EXECUTE FUNCTION invalidate_repertory_text_links();
CREATE TRIGGER repertory_block_changed BEFORE UPDATE OF reviewed_text,review_status ON document_blocks FOR EACH ROW EXECUTE FUNCTION invalidate_repertory_text_links();

CREATE FUNCTION invalidate_repertory_descendants() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE rubric_remedies SET review_status='rejected',validation_note='Rubric context invalidated; review membership again'
 WHERE rubric_id=NEW.id AND review_status IN ('pending','accepted','corrected','unresolved');
 UPDATE structured_entries SET review_status='rejected',validation_note='Parent rubric invalidated; review hierarchy again'
 WHERE parent_id=NEW.id AND review_status IN ('pending','accepted','corrected');
 RETURN NEW;
END $$;
CREATE TRIGGER repertory_entry_rejected AFTER UPDATE OF review_status ON structured_entries
 FOR EACH ROW WHEN (NEW.kind='repertory_rubric' AND NEW.review_status='rejected' AND OLD.review_status<>'rejected')
 EXECUTE FUNCTION invalidate_repertory_descendants();
