-- Acquisitions are independent of ingestion jobs because a reference does not
-- become a source until an exact version has been fetched and validated.
CREATE TABLE paper_acquisition_jobs (
 id uuid PRIMARY KEY, work_id uuid NOT NULL UNIQUE REFERENCES scholarly_works(id),
 status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','review','failed','cancelled')),
 stage text NOT NULL DEFAULT 'resolve', policy_version text NOT NULL,
 candidate_urls_json jsonb NOT NULL, effective_limits_json jsonb NOT NULL,
 attempts int NOT NULL DEFAULT 0, next_retry_at timestamptz,
 lease_until timestamptz, lease_token uuid,
 checkpoint_sha256 char(64), checkpoint_locator text,
 checkpoint_source_url text NOT NULL DEFAULT '', checkpoint_resolver text NOT NULL DEFAULT '',
 source_id uuid REFERENCES sources(id), error_code text NOT NULL DEFAULT '',
 reason text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 CHECK ((checkpoint_sha256 IS NULL)=(checkpoint_locator IS NULL))
);
CREATE INDEX paper_acquisition_jobs_claim_idx ON paper_acquisition_jobs(status,next_retry_at,lease_until,created_at);
