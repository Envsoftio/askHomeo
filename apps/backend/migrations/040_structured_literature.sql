-- ING-06: structured assertions share the existing immutable source asset and
-- processing revision. A category or a text passage alone never verifies them.
CREATE TABLE remedies (
 id uuid PRIMARY KEY,
 canonical_name text NOT NULL CHECK (btrim(canonical_name) <> ''),
 preparation_key text NOT NULL CHECK (btrim(preparation_key) <> ''),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (canonical_name, preparation_key)
);

CREATE TABLE remedy_aliases (
 id uuid PRIMARY KEY,
 spelling text NOT NULL CHECK (btrim(spelling) <> ''),
 remedy_id uuid REFERENCES remedies(id),
 status text NOT NULL CHECK (status IN ('reviewed','ambiguous','unresolved')),
 source_id uuid REFERENCES sources(id),
 processing_revision_id uuid REFERENCES processing_revisions(id),
 review_note text NOT NULL DEFAULT '',
 reviewed_by_principal_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (processing_revision_id,source_id) REFERENCES processing_revisions(id,source_id),
 CHECK ((source_id IS NULL) = (processing_revision_id IS NULL)),
 CHECK ((status='reviewed' AND remedy_id IS NOT NULL) OR
        (status IN ('ambiguous','unresolved') AND remedy_id IS NULL))
);
CREATE INDEX remedy_aliases_lookup_idx ON remedy_aliases (lower(spelling));

CREATE TABLE structured_entries (
 id uuid PRIMARY KEY,
 source_id uuid NOT NULL REFERENCES sources(id),
 source_asset_id uuid NOT NULL REFERENCES source_assets(id),
 processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 kind text NOT NULL CHECK (kind IN ('materia_medica','repertory_rubric')),
 parent_id uuid,
 ordinal int NOT NULL CHECK (ordinal >= 0),
 heading text NOT NULL CHECK (btrim(heading) <> ''),
 full_path text[] NOT NULL CHECK (cardinality(full_path) > 0),
 source_remedy_spelling text NOT NULL DEFAULT '',
 remedy_id uuid REFERENCES remedies(id),
 subsection text NOT NULL DEFAULT '',
 review_status text NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','accepted','corrected','rejected')),
 adapter_version text NOT NULL,
 validation_note text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (processing_revision_id,source_id,source_asset_id)
  REFERENCES processing_revisions(id,source_id,source_asset_id),
 UNIQUE (id,source_id,processing_revision_id),
 UNIQUE (processing_revision_id,kind,ordinal),
 FOREIGN KEY (parent_id,source_id,processing_revision_id)
  REFERENCES structured_entries(id,source_id,processing_revision_id),
 CHECK ((kind='materia_medica' AND parent_id IS NULL AND source_remedy_spelling<>'') OR
        (kind='repertory_rubric' AND source_remedy_spelling=''))
);
CREATE INDEX structured_entries_revision_idx ON structured_entries(source_id,processing_revision_id,kind,review_status);

-- Offsets refer to the reviewed Unicode text of the exact page or block.
-- Multiple rows may support an entry that spans pages or sections.
CREATE TABLE structured_entry_locations (
 entry_id uuid NOT NULL,
 source_id uuid NOT NULL,
 processing_revision_id uuid NOT NULL,
 page_id uuid,
 document_block_id uuid,
 start_character int NOT NULL CHECK (start_character >= 0),
 end_character int NOT NULL CHECK (end_character > start_character),
 exact_text text NOT NULL CHECK (exact_text <> ''),
 FOREIGN KEY (entry_id,source_id,processing_revision_id)
  REFERENCES structured_entries(id,source_id,processing_revision_id),
 FOREIGN KEY (page_id,source_id,processing_revision_id)
  REFERENCES pages(id,source_id,processing_revision_id),
 FOREIGN KEY (document_block_id,source_id,processing_revision_id)
  REFERENCES document_blocks(id,source_id,processing_revision_id),
 CHECK ((page_id IS NULL) <> (document_block_id IS NULL))
);
CREATE UNIQUE INDEX structured_entry_page_location_idx ON structured_entry_locations(entry_id,page_id,start_character) WHERE page_id IS NOT NULL;
CREATE UNIQUE INDEX structured_entry_block_location_idx ON structured_entry_locations(entry_id,document_block_id,start_character) WHERE document_block_id IS NOT NULL;

CREATE TABLE rubric_remedies (
 id uuid PRIMARY KEY,
 rubric_id uuid NOT NULL,
 source_id uuid NOT NULL,
 processing_revision_id uuid NOT NULL,
 remedy_id uuid REFERENCES remedies(id),
 source_notation text NOT NULL CHECK (btrim(source_notation) <> ''),
 source_remedy_spelling text NOT NULL CHECK (btrim(source_remedy_spelling) <> ''),
 grade smallint CHECK (grade > 0),
 grade_scheme text NOT NULL DEFAULT '',
 review_status text NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','accepted','corrected','rejected','unresolved')),
 validation_note text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (rubric_id,source_id,processing_revision_id)
  REFERENCES structured_entries(id,source_id,processing_revision_id),
 CHECK (grade IS NULL OR grade_scheme <> ''),
 CHECK (remedy_id IS NOT NULL OR review_status IN ('pending','unresolved','rejected')),
 UNIQUE (id,source_id,processing_revision_id),
 UNIQUE (rubric_id,source_notation,source_remedy_spelling)
);
CREATE INDEX rubric_remedies_rubric_idx ON rubric_remedies(rubric_id,review_status);

CREATE TABLE rubric_remedy_locations (
 association_id uuid NOT NULL,
 source_id uuid NOT NULL,
 processing_revision_id uuid NOT NULL,
 page_id uuid,
 document_block_id uuid,
 start_character int NOT NULL CHECK (start_character >= 0),
 end_character int NOT NULL CHECK (end_character > start_character),
 exact_text text NOT NULL CHECK (exact_text <> ''),
 FOREIGN KEY (association_id,source_id,processing_revision_id)
  REFERENCES rubric_remedies(id,source_id,processing_revision_id),
 FOREIGN KEY (page_id,source_id,processing_revision_id)
  REFERENCES pages(id,source_id,processing_revision_id),
 FOREIGN KEY (document_block_id,source_id,processing_revision_id)
  REFERENCES document_blocks(id,source_id,processing_revision_id),
 CHECK ((page_id IS NULL) <> (document_block_id IS NULL))
);
CREATE UNIQUE INDEX rubric_remedy_page_location_idx ON rubric_remedy_locations(association_id,page_id,start_character) WHERE page_id IS NOT NULL;
CREATE UNIQUE INDEX rubric_remedy_block_location_idx ON rubric_remedy_locations(association_id,document_block_id,start_character) WHERE document_block_id IS NOT NULL;

CREATE TABLE rubric_cross_references (
 id uuid PRIMARY KEY,
 rubric_id uuid NOT NULL,
 source_id uuid NOT NULL,
 processing_revision_id uuid NOT NULL,
 target_path text[] NOT NULL CHECK (cardinality(target_path)>0),
 target_rubric_id uuid,
 original_notation text NOT NULL CHECK (btrim(original_notation)<>''),
 review_status text NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','accepted','corrected','rejected','unresolved')),
 FOREIGN KEY (rubric_id,source_id,processing_revision_id)
  REFERENCES structured_entries(id,source_id,processing_revision_id),
 FOREIGN KEY (target_rubric_id,source_id,processing_revision_id)
  REFERENCES structured_entries(id,source_id,processing_revision_id),
 UNIQUE (rubric_id,original_notation,target_path)
);

CREATE TABLE structured_review_decisions (
 id uuid PRIMARY KEY,
 source_id uuid NOT NULL REFERENCES sources(id),
 processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 entry_id uuid,
 rubric_remedy_id uuid,
 actor_principal_id uuid,
 decision text NOT NULL CHECK (decision IN ('accepted','corrected','rejected','unresolved')),
 rationale text NOT NULL CHECK (btrim(rationale)<>''),
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((entry_id IS NULL) <> (rubric_remedy_id IS NULL)),
 FOREIGN KEY (entry_id,source_id,processing_revision_id)
  REFERENCES structured_entries(id,source_id,processing_revision_id),
 FOREIGN KEY (rubric_remedy_id,source_id,processing_revision_id)
  REFERENCES rubric_remedies(id,source_id,processing_revision_id)
);

CREATE OR REPLACE FUNCTION check_structured_entry_location() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source_text text;
BEGIN
 IF NEW.page_id IS NOT NULL THEN
  SELECT text_raw INTO source_text FROM pages
   WHERE id=NEW.page_id AND text_qa_status IN ('passed','accepted') AND page_kind='text';
 ELSE
  SELECT reviewed_text INTO source_text FROM document_blocks
   WHERE id=NEW.document_block_id AND review_status IN ('accepted','corrected');
 END IF;
 IF source_text IS NULL OR substring(source_text from NEW.start_character+1 for NEW.end_character-NEW.start_character)<>NEW.exact_text THEN
  RAISE EXCEPTION 'structured location does not match reviewed original text';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER structured_entry_location_exact BEFORE INSERT OR UPDATE ON structured_entry_locations
 FOR EACH ROW EXECUTE FUNCTION check_structured_entry_location();
CREATE TRIGGER rubric_remedy_location_exact BEFORE INSERT OR UPDATE ON rubric_remedy_locations
 FOR EACH ROW EXECUTE FUNCTION check_structured_entry_location();

CREATE OR REPLACE FUNCTION check_structured_review() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_kind text;
BEGIN
 IF NEW.parent_id IS NOT NULL THEN
  SELECT kind INTO parent_kind FROM structured_entries WHERE id=NEW.parent_id;
  IF NEW.kind<>'repertory_rubric' OR parent_kind<>'repertory_rubric' THEN
   RAISE EXCEPTION 'only repertory rubrics may have repertory rubric parents';
  END IF;
 END IF;
 IF NEW.review_status IN ('accepted','corrected') THEN
  IF NEW.kind='materia_medica' AND NEW.remedy_id IS NULL THEN
   RAISE EXCEPTION 'reviewed materia medica entry needs a resolved remedy';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM structured_entry_locations WHERE entry_id=NEW.id) THEN
   RAISE EXCEPTION 'reviewed structured entry needs exact supporting text';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER structured_entry_review_guard BEFORE UPDATE ON structured_entries
 FOR EACH ROW EXECUTE FUNCTION check_structured_review();

CREATE OR REPLACE FUNCTION check_rubric_remedy_review() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE entry_kind text;
BEGIN
 SELECT kind INTO entry_kind FROM structured_entries WHERE id=NEW.rubric_id;
 IF entry_kind<>'repertory_rubric' THEN
  RAISE EXCEPTION 'rubric association must point to a repertory rubric';
 END IF;
 IF NEW.review_status IN ('accepted','corrected') AND NOT EXISTS
  (SELECT 1 FROM rubric_remedy_locations WHERE association_id=NEW.id) THEN
  RAISE EXCEPTION 'reviewed rubric association needs exact supporting text';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER rubric_remedy_review_guard BEFORE UPDATE ON rubric_remedies
 FOR EACH ROW EXECUTE FUNCTION check_rubric_remedy_review();

CREATE OR REPLACE FUNCTION guard_published_structured_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS (SELECT 1 FROM sources WHERE id=OLD.source_id AND published_revision_id=OLD.processing_revision_id) THEN
  RAISE EXCEPTION 'published structured evidence is immutable';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $$;
CREATE TRIGGER structured_entries_published_guard BEFORE UPDATE OR DELETE ON structured_entries
 FOR EACH ROW EXECUTE FUNCTION guard_published_structured_revision();
CREATE TRIGGER rubric_remedies_published_guard BEFORE UPDATE OR DELETE ON rubric_remedies
 FOR EACH ROW EXECUTE FUNCTION guard_published_structured_revision();

CREATE OR REPLACE FUNCTION guard_published_structured_location() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owner_source uuid; owner_revision uuid;
BEGIN
 IF TG_OP='INSERT' THEN owner_source:=NEW.source_id; owner_revision:=NEW.processing_revision_id;
 ELSE owner_source:=OLD.source_id; owner_revision:=OLD.processing_revision_id; END IF;
 IF EXISTS (SELECT 1 FROM sources WHERE id=owner_source AND published_revision_id=owner_revision) THEN
  RAISE EXCEPTION 'published structured location is immutable';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $$;
CREATE TRIGGER structured_entry_location_published_guard BEFORE INSERT OR UPDATE OR DELETE ON structured_entry_locations
 FOR EACH ROW EXECUTE FUNCTION guard_published_structured_location();
CREATE TRIGGER rubric_remedy_location_published_guard BEFORE INSERT OR UPDATE OR DELETE ON rubric_remedy_locations
 FOR EACH ROW EXECUTE FUNCTION guard_published_structured_location();
CREATE TRIGGER rubric_cross_reference_published_guard BEFORE INSERT OR UPDATE OR DELETE ON rubric_cross_references
 FOR EACH ROW EXECUTE FUNCTION guard_published_structured_location();
CREATE TRIGGER structured_review_decisions_immutable BEFORE UPDATE OR DELETE ON structured_review_decisions
 FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();

-- Only verified current records on a published, allowed, ready source are
-- candidates for structured research. Unknown grades remain NULL.
CREATE VIEW eligible_structured_entries AS
 SELECT e.* FROM structured_entries e
 JOIN sources s ON s.id=e.source_id AND s.published_revision_id=e.processing_revision_id
 JOIN active_indexes ai ON ai.source_id=s.id
 JOIN index_runs ir ON ir.id=ai.index_run_id AND ir.status='ready'
 JOIN publications pub ON pub.id=ir.publication_id AND pub.processing_revision_id=e.processing_revision_id
 WHERE e.review_status IN ('accepted','corrected')
 AND s.status='published' AND s.rights_status='allowed' AND s.superseded_at IS NULL
 AND EXISTS (SELECT 1 FROM structured_entry_locations l WHERE l.entry_id=e.id);

CREATE VIEW eligible_rubric_remedies AS
 SELECT rr.* FROM rubric_remedies rr
 JOIN eligible_structured_entries e ON e.id=rr.rubric_id AND e.kind='repertory_rubric'
 WHERE rr.review_status IN ('accepted','corrected') AND rr.remedy_id IS NOT NULL
 AND EXISTS (SELECT 1 FROM rubric_remedy_locations l WHERE l.association_id=rr.id);
