ALTER TABLE pages ADD COLUMN text_revision integer NOT NULL DEFAULT 0;
CREATE TABLE page_text_repairs (
 id uuid PRIMARY KEY,
 page_id uuid NOT NULL REFERENCES pages(id),
 processing_revision_id uuid NOT NULL REFERENCES processing_revisions(id),
 actor_principal_id uuid NOT NULL REFERENCES app_principals(id),
 revision integer NOT NULL,
 previous_text text NOT NULL,
 corrected_text text NOT NULL,
 rationale text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(page_id,revision)
);
CREATE TRIGGER page_text_repairs_immutable BEFORE UPDATE OR DELETE ON page_text_repairs
 FOR EACH ROW EXECUTE FUNCTION guard_immutable_provenance();
-- Recheck unpublished PDFs using word confidence, including previously accepted pages.
UPDATE pages p SET text_qa_at=NULL,text_qa_status='pending',text_qa_reason='Queued for improved scan verification'
 FROM sources s WHERE s.id=p.source_id AND s.status='review' AND s.document_format='pdf' AND p.page_kind='text';
INSERT INTO jobs(id,source_id,kind,status,current_step,total)
 SELECT gen_random_uuid(),s.id,'text_qa','queued','Checking text against scans',count(p.id)::int
 FROM sources s JOIN pages p ON p.source_id=s.id
 WHERE s.status='review' AND s.document_format='pdf' AND p.page_kind='text'
 GROUP BY s.id
 ON CONFLICT(source_id,kind) DO UPDATE SET status='queued',attempts=0,completed=0,total=EXCLUDED.total,error=NULL,lease_until=NULL,run_after=now();
