CREATE TABLE answer_searches (
 answer_id uuid NOT NULL REFERENCES answers(id),
 ordinal int NOT NULL CHECK (ordinal > 0),
 query text NOT NULL CHECK (length(query) BETWEEN 1 AND 1000),
 source_scope text NOT NULL CHECK (source_scope IN ('all','nash','farrington')),
 candidate_count int NOT NULL CHECK (candidate_count >= 0),
 PRIMARY KEY(answer_id,ordinal)
);
ALTER TABLE answer_citations ADD COLUMN evidence_label text NOT NULL DEFAULT '';
UPDATE answer_citations SET evidence_label='E'||ordinal WHERE evidence_label='';
ALTER TABLE answer_citations ADD CONSTRAINT answer_citation_label_check CHECK (evidence_label ~ '^E([1-9]|10)$');
CREATE TABLE answer_index_runs (
 answer_id uuid NOT NULL REFERENCES answers(id),
 index_run_id uuid NOT NULL REFERENCES index_runs(id),
 PRIMARY KEY(answer_id,index_run_id)
);
