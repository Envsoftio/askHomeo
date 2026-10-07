ALTER TABLE sources ADD COLUMN IF NOT EXISTS edition text NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN IF NOT EXISTS publication_info text NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN IF NOT EXISTS repository text NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN IF NOT EXISTS rights_statement text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS sources_pdf_sha_idx ON sources(pdf_sha256);
