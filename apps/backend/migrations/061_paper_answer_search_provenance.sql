-- A saved answer can retain the bounded discovery observation that led to its
-- selected evidence. The search snapshot remains owned by its original user.
ALTER TABLE answer_jobs ADD COLUMN paper_search_id uuid REFERENCES paper_searches(id);
ALTER TABLE answers ADD COLUMN paper_search_id uuid REFERENCES paper_searches(id);
CREATE INDEX answers_paper_search_idx ON answers(paper_search_id) WHERE paper_search_id IS NOT NULL;
