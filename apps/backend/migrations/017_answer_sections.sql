ALTER TABLE answers ADD COLUMN research_sections jsonb NOT NULL DEFAULT '[]'::jsonb
 CHECK (jsonb_typeof(research_sections) = 'array');
