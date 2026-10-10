-- A transcription change must invalidate approvals attached to that text.
CREATE FUNCTION invalidate_mm_page_links() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.text_raw IS DISTINCT FROM OLD.text_raw OR NEW.page_kind IS DISTINCT FROM OLD.page_kind OR NEW.text_qa_status IS DISTINCT FROM OLD.text_qa_status THEN
 UPDATE structured_entries SET review_status='rejected',validation_note='Source page changed; map and review this entry again'
 WHERE kind='materia_medica' AND review_status IN ('pending','accepted','corrected')
 AND id IN (SELECT entry_id FROM structured_entry_locations WHERE page_id=OLD.id);
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER mm_page_changed BEFORE UPDATE OF text_raw,page_kind,text_qa_status ON pages FOR EACH ROW EXECUTE FUNCTION invalidate_mm_page_links();

CREATE FUNCTION invalidate_mm_block_links() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.reviewed_text IS DISTINCT FROM OLD.reviewed_text OR NEW.review_status IS DISTINCT FROM OLD.review_status THEN
 UPDATE structured_entries SET review_status='rejected',validation_note='Source block changed; map and review this entry again'
 WHERE kind='materia_medica' AND review_status IN ('pending','accepted','corrected')
 AND id IN (SELECT entry_id FROM structured_entry_locations WHERE document_block_id=OLD.id);
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER mm_block_changed BEFORE UPDATE OF reviewed_text,review_status ON document_blocks FOR EACH ROW EXECUTE FUNCTION invalidate_mm_block_links();

-- Retrieval only uses chunks entirely inside a reviewed entry span. In
-- particular, a chunk straddling two remedy entries cannot inherit either name.
CREATE VIEW eligible_mm_chunks AS
 SELECT DISTINCT e.id AS entry_id,e.remedy_id,e.source_remedy_spelling,r.canonical_name,r.preparation_key,c.id AS chunk_id
 FROM eligible_structured_entries e JOIN remedies r ON r.id=e.remedy_id
 JOIN structured_entry_locations l ON l.entry_id=e.id
 JOIN chunks c ON c.source_id=e.source_id AND
 ((c.page_id=l.page_id AND l.page_id IS NOT NULL) OR (c.document_block_id=l.document_block_id AND l.document_block_id IS NOT NULL))
 AND c.start_character>=l.start_character AND c.end_character<=l.end_character
 JOIN evidence_locations p ON p.chunk_id=c.id
 WHERE e.kind='materia_medica' AND p.processing_revision_id=e.processing_revision_id
 AND p.page_kind='text' AND p.text_qa_status IN ('passed','accepted')
 AND substring(p.text_raw FROM l.start_character+1 FOR l.end_character-l.start_character)=l.exact_text;

-- Keep the exact reviewed identity context supplied during answer generation.
ALTER TABLE answer_candidates ADD COLUMN remedy_context text NOT NULL DEFAULT '';

-- Apply the no-overlap rule to the pre-existing structured review API too.
CREATE FUNCTION check_mm_approval_overlap() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.kind='materia_medica' AND NEW.review_status IN ('accepted','corrected') AND EXISTS (
 SELECT 1 FROM structured_entry_locations a JOIN structured_entry_locations b
 ON (a.page_id=b.page_id OR a.document_block_id=b.document_block_id)
 AND a.start_character<b.end_character AND b.start_character<a.end_character
 JOIN structured_entries other ON other.id=b.entry_id
 WHERE a.entry_id=NEW.id AND other.id<>NEW.id AND other.kind='materia_medica'
 AND other.processing_revision_id=NEW.processing_revision_id AND other.review_status IN ('accepted','corrected')
 ) THEN RAISE EXCEPTION 'materia medica text already belongs to an approved entry'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER mm_approval_overlap BEFORE UPDATE ON structured_entries FOR EACH ROW EXECUTE FUNCTION check_mm_approval_overlap();
