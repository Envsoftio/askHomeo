CREATE TABLE answer_claims (
 answer_id uuid NOT NULL REFERENCES answers(id),
 ordinal integer NOT NULL,
 claim_text text NOT NULL,
 decision text NOT NULL CHECK (decision IN ('supported','unsupported','irrelevant','missing_excerpt')),
 check_method text NOT NULL,
 checked_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(answer_id,ordinal)
);
CREATE TABLE answer_claim_support (
 answer_id uuid NOT NULL,
 claim_ordinal integer NOT NULL,
 chunk_id uuid NOT NULL REFERENCES chunks(id),
 evidence_label text NOT NULL,
 excerpt_start integer NOT NULL CHECK (excerpt_start>=0),
 excerpt_end integer NOT NULL CHECK (excerpt_end>excerpt_start),
 excerpt_text text NOT NULL,
 PRIMARY KEY(answer_id,claim_ordinal,chunk_id),
 FOREIGN KEY(answer_id,claim_ordinal) REFERENCES answer_claims(answer_id,ordinal)
);
