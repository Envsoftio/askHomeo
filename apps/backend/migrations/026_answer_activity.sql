ALTER TABLE answers DROP CONSTRAINT answers_status_check;
ALTER TABLE answers ADD CONSTRAINT answers_status_check CHECK (status IN ('answered','partial','insufficient_evidence'));

CREATE TABLE answer_jobs (
 id uuid PRIMARY KEY,
 question text NOT NULL,
 research_mode text NOT NULL CHECK (research_mode IN ('quick','deep')),
 source_ids uuid[] NOT NULL DEFAULT '{}',
 status text NOT NULL CHECK (status IN ('waiting','working','retrying','finished','failed')),
 stage text NOT NULL DEFAULT 'Waiting for research',
 attempts integer NOT NULL DEFAULT 0,
 run_after timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz,
 heartbeat_at timestamptz,
 answer_id uuid REFERENCES answers(id),
 error_code text,
 error_message text,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz
);
CREATE INDEX answer_jobs_pending_idx ON answer_jobs(run_after,created_at) WHERE status IN ('waiting','retrying','working');
ALTER TABLE answers ADD COLUMN answer_job_id uuid UNIQUE REFERENCES answer_jobs(id);

CREATE TABLE activity_events (
 id uuid PRIMARY KEY,
 answer_job_id uuid REFERENCES answer_jobs(id),
 kind text NOT NULL,
 message text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 read_at timestamptz,
 CHECK (answer_job_id IS NOT NULL)
);
CREATE INDEX activity_events_created_idx ON activity_events(created_at DESC,id DESC);
