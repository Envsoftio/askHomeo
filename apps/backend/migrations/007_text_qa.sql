ALTER TABLE pages ADD COLUMN text_qa_status text NOT NULL DEFAULT 'pending'
 CHECK (text_qa_status IN ('pending','passed','suspect','accepted'));
ALTER TABLE pages ADD COLUMN text_qa_reason text NOT NULL DEFAULT '';
ALTER TABLE pages ADD COLUMN text_qa_at timestamptz;
CREATE INDEX pages_text_qa_idx ON pages(source_id,text_qa_status) WHERE page_kind='text';
INSERT INTO jobs(id,source_id,kind,status,current_step,total)
SELECT gen_random_uuid(),s.id,'text_qa','queued','Checking text against scans',count(p.id)::int
FROM sources s JOIN pages p ON p.source_id=s.id
WHERE s.status='review' AND p.page_kind='text'
GROUP BY s.id
ON CONFLICT(source_id,kind) DO NOTHING;
