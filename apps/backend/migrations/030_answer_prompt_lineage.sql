ALTER TABLE answers ADD COLUMN prompt_revision text NOT NULL DEFAULT 'legacy-prose-v1';
ALTER TABLE answer_claims ADD COLUMN verification_prompt_revision text NOT NULL DEFAULT 'quote-relevance-v2';
ALTER TABLE answer_claims ADD COLUMN verification_model text NOT NULL DEFAULT '';
