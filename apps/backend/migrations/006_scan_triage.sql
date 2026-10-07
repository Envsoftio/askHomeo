ALTER TABLE pages ADD COLUMN triaged_at timestamptz;
ALTER TABLE pages ADD COLUMN triage_reason text NOT NULL DEFAULT '';
INSERT INTO jobs(id,source_id,kind,status,current_step,total)
SELECT gen_random_uuid(),s.id,'triage','queued','Checking scans without text',count(p.id)::int
FROM sources s JOIN pages p ON p.source_id=s.id
WHERE s.status='review' AND p.scan_page_index>=0 AND length(trim(p.text_raw))<10
GROUP BY s.id
ON CONFLICT(source_id,kind) DO NOTHING;
