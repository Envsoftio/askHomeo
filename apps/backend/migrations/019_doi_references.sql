CREATE TABLE IF NOT EXISTS doi_references (
 id uuid PRIMARY KEY,
 doi text NOT NULL UNIQUE,
 title text NOT NULL,
 authors text NOT NULL,
 publication_year int,
 publisher text NOT NULL DEFAULT '',
 work_type text NOT NULL DEFAULT '',
 doi_url text NOT NULL,
 pdf_url text NOT NULL DEFAULT '',
 license_url text NOT NULL DEFAULT '',
 metadata_provider text NOT NULL,
 source_id uuid UNIQUE REFERENCES sources(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
