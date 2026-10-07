ALTER TABLE answers ADD COLUMN research_mode text NOT NULL DEFAULT 'quick'
 CHECK (research_mode IN ('quick','deep'));
