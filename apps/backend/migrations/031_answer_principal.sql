ALTER TABLE answer_jobs ADD COLUMN owner_role text NOT NULL DEFAULT 'admin' CHECK (owner_role IN ('admin','reviewer'));
ALTER TABLE answers ADD COLUMN owner_role text NOT NULL DEFAULT 'admin' CHECK (owner_role IN ('admin','reviewer'));
CREATE TABLE activity_event_reads (
 event_id uuid NOT NULL REFERENCES activity_events(id),
 actor_role text NOT NULL CHECK (actor_role IN ('admin','reviewer')),
 read_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,actor_role)
);
