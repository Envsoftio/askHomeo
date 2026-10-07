ALTER TABLE doi_references ADD COLUMN hidden_at timestamptz;
ALTER TABLE sources ADD COLUMN pdf_origin_url text NOT NULL DEFAULT '';
