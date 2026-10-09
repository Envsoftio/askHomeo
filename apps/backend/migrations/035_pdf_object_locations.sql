-- Physical location changes must not rewrite immutable source/provenance snapshots.
CREATE TABLE pdf_object_locations (
 sha256 char(64) NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
 bucket text NOT NULL,
 locator text NOT NULL,
 verified_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (sha256,bucket)
);
