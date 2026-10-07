ALTER TABLE answers ADD COLUMN omitted_claim_count integer NOT NULL DEFAULT 0
 CHECK (omitted_claim_count >= 0 AND omitted_claim_count <= 100);
