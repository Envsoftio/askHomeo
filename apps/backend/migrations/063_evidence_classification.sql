ALTER TABLE sources DROP CONSTRAINT sources_evidence_category_check;
ALTER TABLE sources ADD CONSTRAINT sources_evidence_category_check CHECK(evidence_category IN ('unknown','classical_reference','published_case','trial_study','observational_study','study_protocol','systematic_review','trial_registration','guideline_safety','other'));
ALTER TABLE sources ADD COLUMN evidence_category_origin text NOT NULL DEFAULT 'fallback' CHECK(evidence_category_origin IN ('fallback','automatic','manual'));
-- Existing non-unknown values came from review or intake, not this classifier.
UPDATE sources s SET evidence_category_origin='manual'
WHERE s.evidence_category<>'unknown' OR EXISTS (
 SELECT 1 FROM literature_category_decisions d
 WHERE d.source_id=s.id AND d.actor_principal_id IS NOT NULL);
ALTER TABLE literature_category_suggestions ADD COLUMN evidence_category text NOT NULL DEFAULT 'unknown';
ALTER TABLE literature_category_suggestions ADD COLUMN evidence_state text NOT NULL DEFAULT 'uncertain' CHECK(evidence_state IN ('suggested','uncertain'));
ALTER TABLE literature_category_suggestions ADD COLUMN evidence_reason text NOT NULL DEFAULT '';
ALTER TABLE literature_category_suggestions ADD COLUMN evidence_classifier_version text NOT NULL DEFAULT '';
