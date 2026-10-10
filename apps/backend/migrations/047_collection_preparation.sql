-- Durable post-capture preparation. A collection item ID is also its source ID,
-- so a worker restart can retry without creating a second source.
ALTER TABLE collection_items ADD COLUMN preparation_state text NOT NULL DEFAULT 'pending'
 CHECK (preparation_state IN ('pending','running','done','failed'));
ALTER TABLE collection_items ADD COLUMN preparation_attempts int NOT NULL DEFAULT 0 CHECK (preparation_attempts>=0);
ALTER TABLE collection_items ADD COLUMN preparation_error text NOT NULL DEFAULT '';
ALTER TABLE collection_items ADD COLUMN preparation_next_run_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE collection_items ADD COLUMN preparation_lease_until timestamptz;
UPDATE collection_items SET preparation_state='done' WHERE source_id IS NOT NULL;
CREATE INDEX collection_items_prepare_idx ON collection_items(preparation_state,preparation_next_run_at)
 WHERE state='fetched' AND role='content_candidate' AND source_id IS NULL;
