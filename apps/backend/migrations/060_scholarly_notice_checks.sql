-- A notice check is an observation; never erase a previously confirmed notice.
ALTER TABLE scholarly_works ADD COLUMN notice_checked_at timestamptz;
ALTER TABLE scholarly_works ADD COLUMN notice_provider text NOT NULL DEFAULT '';
