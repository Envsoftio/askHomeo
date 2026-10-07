ALTER TABLE answer_jobs ADD COLUMN question_raw text NOT NULL DEFAULT '';
UPDATE answer_jobs SET question_raw=question;

CREATE TABLE answer_job_logs (
 id bigserial PRIMARY KEY,
 answer_job_id uuid NOT NULL REFERENCES answer_jobs(id),
 attempt integer NOT NULL CHECK (attempt >= 0),
 stage text NOT NULL,
 detail jsonb NOT NULL DEFAULT '{}'::jsonb,
 duration_ms bigint CHECK (duration_ms >= 0),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX answer_job_logs_job_idx ON answer_job_logs(answer_job_id,id);
