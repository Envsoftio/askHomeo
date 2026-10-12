ALTER TABLE scholarly_works ADD COLUMN notice_type text NOT NULL DEFAULT '';
CREATE TABLE scholarly_notices (
 work_id uuid NOT NULL REFERENCES scholarly_works(id),
 notice_doi text NOT NULL, notice_type text NOT NULL,
 notice_url text NOT NULL, provider text NOT NULL,
 first_checked_at timestamptz NOT NULL DEFAULT now(),
 last_checked_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(work_id,notice_doi,notice_type)
);
