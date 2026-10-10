-- Visual notation and categorical conventions are independent of numeric grades.
-- Existing records remain unknown; no style or numeric conversion is inferred.
ALTER TABLE rubric_remedies
 ADD COLUMN source_style text NOT NULL DEFAULT 'unknown'
  CHECK(source_style IN ('unknown','ordinary','italic','bold','bold_italic','other')),
 ADD COLUMN categorical_grade text NOT NULL DEFAULT ''
  CHECK(length(categorical_grade)<=300 AND (categorical_grade='' OR btrim(grade_scheme)<>''));

CREATE FUNCTION check_repertory_notation_review() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.review_status IN ('accepted','corrected') THEN
  IF NEW.source_style<>'unknown' AND btrim(NEW.validation_note)='' THEN
   RAISE EXCEPTION 'reviewed typography requires an original-source review note';
  END IF;
  IF NEW.categorical_grade<>'' AND NOT EXISTS (
   SELECT 1 FROM rubric_remedy_locations l WHERE l.association_id=NEW.id
   AND strpos(l.exact_text,NEW.grade_scheme)>0
  ) THEN
   RAISE EXCEPTION 'categorical grade requires exact source convention evidence';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER repertory_notation_review_guard BEFORE INSERT OR UPDATE ON rubric_remedies
 FOR EACH ROW EXECUTE FUNCTION check_repertory_notation_review();

CREATE OR REPLACE VIEW eligible_rubric_remedies AS
 SELECT rr.* FROM rubric_remedies rr
 JOIN eligible_structured_entries e ON e.id=rr.rubric_id AND e.kind='repertory_rubric'
 WHERE rr.review_status IN ('accepted','corrected') AND rr.remedy_id IS NOT NULL
 AND EXISTS (SELECT 1 FROM rubric_remedy_locations l WHERE l.association_id=rr.id);
