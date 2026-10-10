-- Existing databases may have applied 041 before capture leases gained an
-- ownership token. The original migration is immutable once recorded.
ALTER TABLE collection_capture_jobs
 ADD COLUMN IF NOT EXISTS lease_token uuid;
