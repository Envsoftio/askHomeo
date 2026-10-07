CREATE TABLE answer_candidates (
 answer_id uuid NOT NULL REFERENCES answers(id),
 ordinal int NOT NULL CHECK (ordinal BETWEEN 1 AND 10),
 chunk_id uuid NOT NULL REFERENCES chunks(id),
 retrieval_score double precision NOT NULL CHECK (retrieval_score >= 0 AND retrieval_score < 1000),
 PRIMARY KEY(answer_id,ordinal), UNIQUE(answer_id,chunk_id)
);
