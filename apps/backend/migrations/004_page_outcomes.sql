ALTER TABLE pages ADD COLUMN IF NOT EXISTS page_kind text NOT NULL DEFAULT 'text';
ALTER TABLE pages ADD CONSTRAINT pages_kind_check CHECK (page_kind IN ('text','unclassified','blank','illustration','book_info','missing_text'));
UPDATE pages SET page_kind='unclassified' WHERE scan_page_index>=0 AND length(trim(text_raw))<10 AND review_status='needs_review';
UPDATE pages SET page_kind='book_info' WHERE scan_page_index=-1;
CREATE INDEX IF NOT EXISTS pages_source_kind_idx ON pages(source_id,page_kind,scan_page_index);
