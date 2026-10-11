-- Re-evaluate only unresolved suspect pages with the less brittle OCR comparison.
UPDATE pages p SET text_qa_at=NULL,text_qa_status='pending',
 text_qa_reason='Queued for improved text verification'
FROM sources s
WHERE s.id=p.source_id AND s.status='review' AND s.document_format='pdf'
 AND p.page_kind='text' AND p.text_qa_status='suspect';

INSERT INTO jobs(id,source_id,kind,status,current_step,total)
SELECT gen_random_uuid(),s.id,'text_qa','queued','Rechecking suspect pages',
 (SELECT count(*) FROM pages p WHERE p.source_id=s.id AND p.page_kind='text')::int
FROM sources s
WHERE s.status='review' AND s.document_format='pdf'
 AND EXISTS (SELECT 1 FROM pages p WHERE p.source_id=s.id AND p.page_kind='text' AND p.text_qa_at IS NULL)
ON CONFLICT(source_id,kind) DO UPDATE SET status='queued',attempts=0,completed=0,
 total=EXCLUDED.total,error=NULL,lease_until=NULL,run_after=now();
