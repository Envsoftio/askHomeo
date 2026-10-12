-- A PMID-only work cannot enter doi_references: its DOI is required and unique.
-- Scholarly references remain metadata, never evidence until linked to a source.
CREATE TABLE scholarly_works (
 id uuid PRIMARY KEY, doi text, pmid text, pmcid text,
 title text NOT NULL, authors text NOT NULL DEFAULT '', publication_year int,
 publication_type text NOT NULL DEFAULT '', abstract text NOT NULL DEFAULT '',
 study_country text NOT NULL DEFAULT 'unknown', study_country_review_status text NOT NULL DEFAULT 'pending',
 author_affiliation_country text NOT NULL DEFAULT 'unknown', publication_country text NOT NULL DEFAULT 'unknown',
 access_status text NOT NULL DEFAULT 'unknown', rights_status text NOT NULL DEFAULT 'pending',
 processing_status text NOT NULL DEFAULT 'not_started', notice_url text NOT NULL DEFAULT '',
 source_id uuid REFERENCES sources(id), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (doi IS NOT NULL OR pmid IS NOT NULL OR pmcid IS NOT NULL),
 CHECK (study_country_review_status IN ('pending','reviewed'))
);
CREATE UNIQUE INDEX scholarly_works_doi_unique ON scholarly_works(doi) WHERE doi IS NOT NULL;
CREATE UNIQUE INDEX scholarly_works_pmid_unique ON scholarly_works(pmid) WHERE pmid IS NOT NULL;
CREATE UNIQUE INDEX scholarly_works_pmcid_unique ON scholarly_works(pmcid) WHERE pmcid IS NOT NULL;
CREATE TABLE paper_searches (
 id uuid PRIMARY KEY, owner_principal_id uuid NOT NULL, query text NOT NULL,
 providers text[] NOT NULL, state text NOT NULL, cap int NOT NULL,
 results_json jsonb NOT NULL DEFAULT '[]', provider_status_json jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE scholarly_observations (
 id uuid PRIMARY KEY, work_id uuid NOT NULL REFERENCES scholarly_works(id), provider text NOT NULL,
 native_id text NOT NULL, raw_metadata_json jsonb NOT NULL,
 retrieved_at timestamptz NOT NULL DEFAULT now(), search_id uuid REFERENCES paper_searches(id), result_order int,
 UNIQUE(search_id,provider,native_id)
);
CREATE TABLE paper_saved_references (
 owner_principal_id uuid NOT NULL, work_id uuid NOT NULL REFERENCES scholarly_works(id),
 saved_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(owner_principal_id,work_id)
);
CREATE TABLE paper_acquisition_attempts (
 id uuid PRIMARY KEY, work_id uuid NOT NULL REFERENCES scholarly_works(id),
 resolver text NOT NULL, candidate_url text NOT NULL, state text NOT NULL,
 reason text NOT NULL DEFAULT '', response_sha256 text NOT NULL DEFAULT '',
 started_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE(work_id,resolver,candidate_url)
);
ALTER TABLE sources DROP CONSTRAINT sources_document_format_check;
ALTER TABLE sources ADD CONSTRAINT sources_document_format_check CHECK (document_format IN ('pdf','html','txt','jats'));
ALTER TABLE sources DROP CONSTRAINT sources_format_storage_check;
ALTER TABLE sources ADD CONSTRAINT sources_format_storage_check CHECK (
 (document_format='pdf' AND pdf_path IS NOT NULL AND pdf_sha256 IS NOT NULL AND pdf_bytes IS NOT NULL AND page_count IS NOT NULL)
 OR (document_format IN ('html','txt','jats') AND pdf_path IS NULL AND pdf_sha256 IS NULL AND pdf_bytes IS NULL AND page_count IS NULL
 AND document_object_locator IS NOT NULL AND document_sha256 IS NOT NULL AND document_bytes>0));
ALTER TABLE source_assets DROP CONSTRAINT source_assets_kind_check;
ALTER TABLE source_assets ADD CONSTRAINT source_assets_kind_check CHECK (kind IN ('pdf','html','txt','xml','jats'));
ALTER TABLE source_assets DROP CONSTRAINT source_assets_location_check;
ALTER TABLE source_assets ADD CONSTRAINT source_assets_location_check CHECK (
 (kind='pdf' AND pdf_path IS NOT NULL AND object_locator IS NULL) OR
 (kind IN ('html','txt','xml','jats') AND pdf_path IS NULL AND object_locator IS NOT NULL));
