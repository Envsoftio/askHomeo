CREATE TABLE source_access_decisions (
 id uuid PRIMARY KEY,
 source_id uuid NOT NULL REFERENCES sources(id),
 action text NOT NULL CHECK (action IN ('disable','enable')),
 previous_status text NOT NULL,
 reason text NOT NULL,
 actor_role text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX source_access_decisions_source_idx ON source_access_decisions(source_id,created_at DESC);
