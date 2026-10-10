-- Record automatic category prefills separately from human acceptance.
-- Existing manual decisions, publication gates and evidence types are unchanged.
ALTER TABLE literature_category_decisions DROP CONSTRAINT literature_category_decisions_origin_check;
ALTER TABLE literature_category_decisions ADD CONSTRAINT literature_category_decisions_origin_check
 CHECK (origin IN ('accepted_suggestion','manual','detected'));
