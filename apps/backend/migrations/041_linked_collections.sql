-- ING-06 slice A. Collection snapshots are candidates until all selected
-- content and rights have been reviewed. Existing single-document sources and
-- saved answers keep their source/asset/revision identities unchanged.
CREATE TABLE linked_collections (
 id uuid PRIMARY KEY,
 title text NOT NULL CHECK (btrim(title)<>''),
 author text NOT NULL CHECK (btrim(author)<>''),
 edition text NOT NULL DEFAULT '',
 created_by_principal_id uuid,
 created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE collection_snapshots (
 id uuid PRIMARY KEY,
 collection_id uuid NOT NULL REFERENCES linked_collections(id),
 generation int NOT NULL CHECK (generation>0),
 scope_json jsonb NOT NULL,
 state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','capturing','review','failed','cancelled','active')),
 incomplete_reasons text[] NOT NULL DEFAULT '{}',
 bytes_fetched bigint NOT NULL DEFAULT 0 CHECK (bytes_fetched>=0),
 started_at timestamptz,
 finished_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(collection_id,generation),
 UNIQUE(id,collection_id)
);
CREATE INDEX collection_snapshots_collection_idx ON collection_snapshots(collection_id,generation DESC);

CREATE TABLE collection_items (
 id uuid PRIMARY KEY,
 snapshot_id uuid NOT NULL REFERENCES collection_snapshots(id),
 requested_url text NOT NULL,
 final_url text NOT NULL DEFAULT '',
 depth int NOT NULL CHECK(depth>=0),
 discovery_ordinal int NOT NULL CHECK(discovery_ordinal>=0),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','fetching','fetched','failed','excluded','duplicate_redirect')),
 role text NOT NULL DEFAULT 'unresolved' CHECK(role IN ('unresolved','index_only','content_candidate')),
 content_type text NOT NULL DEFAULT '',
 format text NOT NULL DEFAULT '' CHECK(format IN ('','html','txt')),
 sha256 char(64),
 byte_size bigint CHECK(byte_size>=0),
 object_locator text,
 anchors text[] NOT NULL DEFAULT '{}',
 block_count int NOT NULL DEFAULT 0 CHECK(block_count>=0),
 warnings text[] NOT NULL DEFAULT '{}',
 error text NOT NULL DEFAULT '',
 attempts int NOT NULL DEFAULT 0 CHECK(attempts>=0),
 source_id uuid REFERENCES sources(id),
 fetched_at timestamptz,
 UNIQUE(snapshot_id,requested_url),
 UNIQUE(snapshot_id,discovery_ordinal),
 UNIQUE(id,snapshot_id),
 CHECK (state NOT IN ('fetched','duplicate_redirect') OR
  (sha256 IS NOT NULL AND byte_size IS NOT NULL AND object_locator IS NOT NULL AND final_url<>''))
);
CREATE INDEX collection_items_frontier_idx ON collection_items(snapshot_id,state,discovery_ordinal);

CREATE TABLE collection_links (
 id uuid PRIMARY KEY,
 snapshot_id uuid NOT NULL REFERENCES collection_snapshots(id),
 from_item_id uuid NOT NULL,
 ordinal int NOT NULL CHECK(ordinal>=0),
 target_url text NOT NULL,
 fragment text NOT NULL DEFAULT '',
 label text NOT NULL DEFAULT '',
 relation text NOT NULL CHECK(relation IN ('linked_document','index','section','pagination','cross_reference','continuation')),
 state text NOT NULL CHECK(state IN ('queued','resolved','duplicate_or_cycle','excluded_scope','excluded_depth','excluded_document_limit','missing_anchor','target_failed','unfetched')),
 FOREIGN KEY(from_item_id,snapshot_id) REFERENCES collection_items(id,snapshot_id),
 UNIQUE(from_item_id,ordinal)
);
CREATE INDEX collection_links_target_idx ON collection_links(snapshot_id,target_url);

CREATE TABLE collection_capture_jobs (
 snapshot_id uuid PRIMARY KEY REFERENCES collection_snapshots(id),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','done','failed','cancelled')),
 lease_until timestamptz,
 lease_token uuid,
 next_run_at timestamptz NOT NULL DEFAULT now(),
 attempts int NOT NULL DEFAULT 0 CHECK(attempts>=0),
 last_error text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX collection_capture_jobs_claim_idx ON collection_capture_jobs(state,next_run_at);

CREATE TABLE collection_scope_decisions (
 id uuid PRIMARY KEY,
 snapshot_id uuid NOT NULL REFERENCES collection_snapshots(id),
 actor_principal_id uuid,
 decision text NOT NULL CHECK(decision IN ('created','resumed','cancelled','replaced')),
 rationale text NOT NULL CHECK(btrim(rationale)<>''),
 scope_json jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER collection_scope_decisions_immutable BEFORE UPDATE OR DELETE ON collection_scope_decisions
 FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();

CREATE TABLE active_collection_snapshots (
 collection_id uuid PRIMARY KEY REFERENCES linked_collections(id),
 snapshot_id uuid NOT NULL UNIQUE,
 activated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(snapshot_id,collection_id) REFERENCES collection_snapshots(id,collection_id)
);
