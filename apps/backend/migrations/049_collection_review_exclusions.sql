-- A content candidate can be deliberately emptied in the book editor. Keep
-- its saved asset/source for audit without demanding an index for no text.
ALTER TABLE collection_items ADD COLUMN review_excluded boolean NOT NULL DEFAULT false;

CREATE TABLE collection_item_review_decisions (
 id uuid PRIMARY KEY,
 item_id uuid NOT NULL,
 snapshot_id uuid NOT NULL,
 actor_principal_id uuid,
 excluded boolean NOT NULL,
 rationale text NOT NULL CHECK (btrim(rationale)<>''),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(item_id,snapshot_id) REFERENCES collection_items(id,snapshot_id)
);
CREATE TRIGGER collection_item_review_decisions_immutable BEFORE UPDATE OR DELETE ON collection_item_review_decisions
 FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();
