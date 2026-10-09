CREATE OR REPLACE FUNCTION valid_literature_categories(categories text[]) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
 SELECT categories IS NOT NULL AND cardinality(categories)>0
 AND categories <@ ARRAY['materia_medica','repertory','organon_philosophy','therapeutics','provings','clinical_cases','research','other','unclassified']::text[]
 AND cardinality(categories)=(SELECT count(DISTINCT value) FROM unnest(categories) value)
 AND (cardinality(categories)=1 OR NOT 'unclassified'=ANY(categories));
$$;
ALTER TABLE sources ADD COLUMN literature_categories text[] NOT NULL DEFAULT ARRAY['unclassified']::text[];
ALTER TABLE sources ADD CONSTRAINT sources_literature_categories_check CHECK(valid_literature_categories(literature_categories));
ALTER TABLE sources ADD COLUMN literature_category_origin text NOT NULL DEFAULT 'fallback' CHECK(literature_category_origin IN ('fallback','automatic','manual'));
ALTER TABLE sources ADD COLUMN evidence_category text NOT NULL DEFAULT 'unknown' CHECK(evidence_category IN ('unknown','classical_reference','published_case','trial_study','systematic_review','trial_registration','guideline_safety','other'));

CREATE TABLE literature_category_suggestions (
 processing_revision_id uuid PRIMARY KEY REFERENCES processing_revisions(id),
 source_id uuid NOT NULL REFERENCES sources(id), source_asset_id uuid NOT NULL REFERENCES source_assets(id),
 classifier_version text NOT NULL, categories text[] NOT NULL CHECK(valid_literature_categories(categories)),
 state text NOT NULL CHECK(state IN ('suggested','uncertain','failed')),
 reason text NOT NULL, evidence jsonb NOT NULL DEFAULT '[]'::jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(processing_revision_id,source_id,source_asset_id) REFERENCES processing_revisions(id,source_id,source_asset_id)
);
CREATE TABLE literature_category_decisions (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id), processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 actor_principal_id uuid, previous_categories text[] NOT NULL, chosen_categories text[] NOT NULL CHECK(valid_literature_categories(chosen_categories)),
 previous_evidence_category text NOT NULL, chosen_evidence_category text NOT NULL,
 origin text NOT NULL CHECK(origin IN ('accepted_suggestion','manual')),
 rationale text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE literature_section_categories (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id), processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 document_block_id uuid REFERENCES document_blocks(id), page_id uuid REFERENCES pages(id),
 categories text[] NOT NULL CHECK(valid_literature_categories(categories)),
 actor_principal_id uuid, rationale text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((document_block_id IS NULL) <> (page_id IS NULL)),
 UNIQUE(document_block_id), UNIQUE(page_id)
);
CREATE TABLE literature_section_category_decisions (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES sources(id), processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 document_block_id uuid REFERENCES document_blocks(id), page_id uuid REFERENCES pages(id),
 previous_categories text[], chosen_categories text[] NOT NULL CHECK(valid_literature_categories(chosen_categories)),
 actor_principal_id uuid, rationale text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((document_block_id IS NULL) <> (page_id IS NULL))
);
ALTER TABLE answer_jobs ADD COLUMN literature_category_scope text[] NOT NULL DEFAULT '{}'::text[];
ALTER TABLE answers ADD COLUMN literature_category_scope text[] NOT NULL DEFAULT '{}'::text[];
ALTER TABLE answers ADD COLUMN literature_category_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE OR REPLACE VIEW evidence_categories AS
 SELECT p.chunk_id,p.source_id,p.processing_revision_id,
 coalesce(sc.categories,s.literature_categories) AS categories,
 CASE WHEN sc.id IS NULL THEN 'source' ELSE 'section' END AS category_level
 FROM evidence_locations p JOIN sources s ON s.id=p.source_id
 LEFT JOIN literature_section_categories sc ON sc.processing_revision_id=p.processing_revision_id
 AND ((p.format='pdf' AND sc.page_id=p.id) OR (p.format<>'pdf' AND sc.document_block_id=p.id));
