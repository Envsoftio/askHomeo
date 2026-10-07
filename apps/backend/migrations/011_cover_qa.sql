UPDATE pages SET page_kind='book_info', review_status='excluded'
WHERE scan_page_index=-1 AND extraction_method='generated cover' AND page_kind='text';
UPDATE jobs j SET total=(SELECT count(*) FROM pages p WHERE p.source_id=j.source_id AND p.scan_page_index>=0 AND p.page_kind='text'), updated_at=now()
WHERE j.kind='text_qa' AND j.status IN ('queued','running','failed');
