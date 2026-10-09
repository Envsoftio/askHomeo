-- Browser sessions are independent of long-lived API/reviewer bearer tokens.
CREATE TABLE browser_sessions (
    token_hash text PRIMARY KEY,
    principal_id uuid NOT NULL REFERENCES app_principals(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT browser_sessions_token_hash_length CHECK (length(token_hash) = 64),
    CONSTRAINT browser_sessions_expiry CHECK (expires_at > created_at)
);
CREATE INDEX browser_sessions_principal_active_idx ON browser_sessions(principal_id, expires_at) WHERE revoked_at IS NULL;
