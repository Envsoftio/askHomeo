-- The active pointer is the source of truth for collection replacement.
-- Standalone documents and collections without an active snapshot retain their
-- existing publication/index eligibility. Historical citations are unaffected.
CREATE FUNCTION collection_source_retrieval_eligible(candidate_source_id uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS (
  SELECT 1 FROM collection_items i
  JOIN collection_snapshots s ON s.id=i.snapshot_id
  JOIN active_collection_snapshots active ON active.collection_id=s.collection_id
  WHERE i.source_id=candidate_source_id AND active.snapshot_id<>i.snapshot_id
 );
$$;

CREATE TABLE collection_activation_decisions (
 id uuid PRIMARY KEY,
 collection_id uuid NOT NULL REFERENCES linked_collections(id),
 snapshot_id uuid NOT NULL,
 previous_snapshot_id uuid,
 actor_principal_id uuid,
 rationale text NOT NULL CHECK (btrim(rationale)<>''),
 partial boolean NOT NULL,
 activated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(snapshot_id,collection_id) REFERENCES collection_snapshots(id,collection_id),
 FOREIGN KEY(previous_snapshot_id,collection_id) REFERENCES collection_snapshots(id,collection_id)
);
CREATE TRIGGER collection_activation_decisions_immutable BEFORE UPDATE OR DELETE ON collection_activation_decisions
 FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();

CREATE OR REPLACE VIEW eligible_structured_entries AS
 SELECT e.* FROM structured_entries e
 JOIN sources s ON s.id=e.source_id AND s.published_revision_id=e.processing_revision_id
 JOIN active_indexes ai ON ai.source_id=s.id
 JOIN index_runs ir ON ir.id=ai.index_run_id AND ir.status='ready'
 JOIN publications pub ON pub.id=ir.publication_id AND pub.processing_revision_id=e.processing_revision_id
 WHERE e.review_status IN ('accepted','corrected')
 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL
 AND collection_source_retrieval_eligible(s.id)
 AND EXISTS (SELECT 1 FROM structured_entry_locations l WHERE l.entry_id=e.id);
