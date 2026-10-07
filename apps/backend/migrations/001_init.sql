CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS sources (
 id uuid PRIMARY KEY, source_key text NOT NULL UNIQUE, title text NOT NULL, author text NOT NULL,
 publication_year int, source_url text, pdf_path text NOT NULL, pdf_sha256 char(64) NOT NULL,
 pdf_bytes bigint NOT NULL CHECK (pdf_bytes > 0), page_count int NOT NULL CHECK (page_count > 0),
 status text NOT NULL CHECK (status IN ('queued','processing','review','approved','published','failed','disabled')),
 rights_status text NOT NULL DEFAULT 'needs_review' CHECK (rights_status IN ('needs_review','allowed','denied')),
 review_note text, reviewed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS jobs (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id), kind text NOT NULL,
 status text NOT NULL CHECK (status IN ('queued','running','done','failed')),
 current_step text NOT NULL, completed int NOT NULL DEFAULT 0, total int NOT NULL CHECK (total > 0),
 attempts int NOT NULL DEFAULT 0, run_after timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz, error text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(source_id,kind)
);
CREATE TABLE IF NOT EXISTS pages (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id), pdf_page_index int NOT NULL CHECK (pdf_page_index >= 0),
 scan_page_index int NOT NULL CHECK (scan_page_index >= 0), printed_label text, canvas_id text NOT NULL,
 alto_url text NOT NULL, image_url text NOT NULL, text_raw text NOT NULL, text_sha256 char(64) NOT NULL,
 extraction_method text NOT NULL, review_status text NOT NULL DEFAULT 'needs_review',
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(source_id,pdf_page_index), UNIQUE(source_id,canvas_id)
);
CREATE TABLE IF NOT EXISTS chunks (
 id uuid PRIMARY KEY, page_id uuid NOT NULL REFERENCES pages(id), source_id uuid NOT NULL REFERENCES sources(id),
 chunk_index int NOT NULL, text_exact text NOT NULL, start_character int NOT NULL, end_character int NOT NULL,
 search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english',text_exact)) STORED,
 embedding vector, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(page_id,chunk_index), CHECK (start_character >= 0 AND end_character > start_character)
);
CREATE INDEX IF NOT EXISTS chunks_search_idx ON chunks USING gin(search_vector);
CREATE TABLE IF NOT EXISTS answers (
 id uuid PRIMARY KEY, question text NOT NULL, status text NOT NULL CHECK (status IN ('answered','insufficient_evidence')),
 answer_text text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS answer_citations (
 id uuid PRIMARY KEY, answer_id uuid NOT NULL REFERENCES answers(id), chunk_id uuid NOT NULL REFERENCES chunks(id),
 ordinal int NOT NULL, UNIQUE(answer_id,ordinal)
);
