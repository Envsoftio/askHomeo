ALTER TABLE pages DROP CONSTRAINT IF EXISTS pages_scan_page_index_check;
ALTER TABLE pages ADD CONSTRAINT pages_scan_page_index_check CHECK (scan_page_index >= -1);
ALTER TABLE pages ADD COLUMN IF NOT EXISTS review_note text;
INSERT INTO pages(id,source_id,pdf_page_index,scan_page_index,canvas_id,alto_url,image_url,text_raw,text_sha256,extraction_method,review_status)
SELECT gen_random_uuid(),id,0,-1,'','','','','e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855','generated cover','excluded'
FROM sources WHERE source_key='nash-1899-b20409497' ON CONFLICT(source_id,pdf_page_index) DO NOTHING;
