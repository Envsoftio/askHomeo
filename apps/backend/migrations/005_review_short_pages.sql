-- Earlier page checks did not record whether a near-empty scan was blank,
-- illustrated, or had missing OCR. Preserve the note and request an outcome.
UPDATE pages
SET page_kind='unclassified', review_status='needs_review'
WHERE scan_page_index>=0 AND length(trim(text_raw))<10 AND page_kind='text';
