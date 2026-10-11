-- Earlier local imports could keep the default text kind after blank triage.
-- Repair only undecided pages; preserve every explicit reviewer decision.
UPDATE pages p SET page_kind='blank',review_status='auto_checked'
FROM sources s
WHERE s.id=p.source_id AND s.status='review' AND s.document_format='pdf'
 AND p.triaged_at IS NOT NULL AND p.review_status='needs_review'
 AND p.page_kind IN ('text','unclassified') AND length(trim(p.text_raw))<10
 AND p.triage_reason='No meaningful marks detected in the rendered scan; automatically omitted from answers.';

UPDATE pages p SET page_kind='unclassified'
FROM sources s
WHERE s.id=p.source_id AND s.status='review' AND s.document_format='pdf'
 AND p.triaged_at IS NOT NULL AND p.review_status='needs_review'
 AND p.page_kind='text' AND length(trim(p.text_raw))<10
 AND p.triage_reason='Visual content detected; check whether this is a cover, diagram, book information, or missing text.';
