-- A model call can precede answer creation or fail before a job completes. Existing
-- answer logs cannot represent embedding calls or individual provider retries.
CREATE TABLE model_calls (
 id bigserial PRIMARY KEY,
 owner_kind text NOT NULL DEFAULT '' CHECK (owner_kind IN ('', 'answer_job', 'embedding_job')),
 owner_id uuid,
 kind text NOT NULL CHECK (kind IN ('chat', 'embedding')),
 provider text NOT NULL,
 requested_model text NOT NULL,
 returned_model text NOT NULL DEFAULT '',
 provider_request_id text NOT NULL DEFAULT '',
 outcome text NOT NULL,
 prompt_tokens bigint CHECK (prompt_tokens >= 0),
 completion_tokens bigint CHECK (completion_tokens >= 0),
 total_tokens bigint CHECK (total_tokens >= 0),
 reasoning_tokens bigint CHECK (reasoning_tokens >= 0),
 estimated_cost_usd numeric(16,10) CHECK (estimated_cost_usd >= 0),
 cost_origin text NOT NULL DEFAULT 'unknown' CHECK (cost_origin IN ('unknown', 'provider_reported')),
 duration_ms bigint NOT NULL CHECK (duration_ms >= 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((owner_kind = '') = (owner_id IS NULL))
);
CREATE INDEX model_calls_owner_idx ON model_calls(owner_kind, owner_id, id);
