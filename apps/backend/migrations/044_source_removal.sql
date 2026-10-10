-- Removal is a tombstone: immutable source assets, passages and saved-answer
-- provenance remain available for audit, but the source leaves the library and
-- cannot be restored or republished by a background job.
ALTER TABLE sources ADD COLUMN removed_at timestamptz;
CREATE INDEX sources_visible_recent_idx ON sources(created_at DESC) WHERE removed_at IS NULL;

CREATE TABLE source_removals (
 source_id uuid PRIMARY KEY REFERENCES sources(id),
 reason text NOT NULL,
 previous_status text NOT NULL,
 actor_principal_id uuid NOT NULL REFERENCES app_principals(id),
 removed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER source_removals_immutable BEFORE UPDATE OR DELETE ON source_removals
 FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();

CREATE OR REPLACE FUNCTION guard_removed_source_status() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.removed_at IS NOT NULL AND NEW.status <> 'disabled' THEN
  RAISE EXCEPTION 'removed source cannot be restored';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER sources_removed_status_guard BEFORE UPDATE OF status ON sources
 FOR EACH ROW EXECUTE FUNCTION guard_removed_source_status();
