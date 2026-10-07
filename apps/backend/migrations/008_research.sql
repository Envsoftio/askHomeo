CREATE TABLE embedding_configs (
 id uuid PRIMARY KEY, model_id text NOT NULL, model_revision text NOT NULL,
 dimensions int NOT NULL CHECK (dimensions BETWEEN 1 AND 4096),
 distance_metric text NOT NULL DEFAULT 'cosine' CHECK (distance_metric='cosine'),
 preprocessing_version text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(model_id,model_revision,dimensions,preprocessing_version)
);
CREATE TABLE publications (
 id uuid PRIMARY KEY, source_id uuid NOT NULL UNIQUE REFERENCES sources(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE index_runs (
 id uuid PRIMARY KEY, publication_id uuid NOT NULL REFERENCES publications(id),
 embedding_config_id uuid NOT NULL REFERENCES embedding_configs(id),
 status text NOT NULL CHECK (status IN ('pending','running','ready','failed')),
 expected_chunk_count int NOT NULL CHECK (expected_chunk_count>0),
 indexed_chunk_count int NOT NULL DEFAULT 0 CHECK (indexed_chunk_count>=0),
 error text NOT NULL DEFAULT '', started_at timestamptz, finished_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(publication_id,embedding_config_id)
);
CREATE TABLE chunk_embeddings (
 chunk_id uuid NOT NULL REFERENCES chunks(id), embedding_config_id uuid NOT NULL REFERENCES embedding_configs(id),
 embedding vector NOT NULL, input_hash char(64) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(chunk_id,embedding_config_id)
);
CREATE TABLE active_indexes (
 source_id uuid PRIMARY KEY REFERENCES sources(id), index_run_id uuid NOT NULL UNIQUE REFERENCES index_runs(id)
);
CREATE INDEX chunk_embeddings_config_idx ON chunk_embeddings(embedding_config_id,chunk_id);
ALTER TABLE answers ADD COLUMN answer_model text NOT NULL DEFAULT '';
ALTER TABLE answers ADD COLUMN index_run_id uuid REFERENCES index_runs(id);
ALTER TABLE answers ADD COLUMN evidence_ids uuid[] NOT NULL DEFAULT '{}';
CREATE OR REPLACE FUNCTION guard_chunk_embedding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n int;
BEGIN
 SELECT dimensions INTO n FROM embedding_configs WHERE id=NEW.embedding_config_id;
 IF vector_dims(NEW.embedding)<>n THEN RAISE EXCEPTION 'embedding dimension mismatch'; END IF;
 IF vector_norm(NEW.embedding)=0 THEN RAISE EXCEPTION 'zero embedding'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER chunk_embedding_guard BEFORE INSERT OR UPDATE ON chunk_embeddings FOR EACH ROW EXECUTE FUNCTION guard_chunk_embedding();

CREATE TABLE embedding_jobs (job_id uuid PRIMARY KEY REFERENCES jobs(id), index_run_id uuid NOT NULL UNIQUE REFERENCES index_runs(id));
