-- Study location is a reviewed finding from the saved source, independent of
-- author affiliations and journal country.
ALTER TABLE scholarly_works ADD COLUMN study_country_evidence_block_id uuid REFERENCES document_blocks(id);
ALTER TABLE scholarly_works ADD COLUMN study_country_reviewed_by uuid;
ALTER TABLE scholarly_works ADD COLUMN study_country_reviewed_at timestamptz;
ALTER TABLE scholarly_works ADD COLUMN study_country_reason text NOT NULL DEFAULT '';
