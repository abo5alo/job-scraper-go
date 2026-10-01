-- Indexes no query uses. Every one still has to be updated on every write,
-- and each scrape rewrites thousands of rows.

-- Nothing filters on tags; skill filters use the skills column.
DROP INDEX IF EXISTS jobs_tags_idx;
-- The only query on last_seen_at (closing missing jobs) narrows by board
-- first, which jobs_board_idx covers.
DROP INDEX IF EXISTS jobs_last_seen_at_idx;
-- A DESC index keeps NULLs first, but search sorts by
-- "posted_at DESC NULLS LAST", so this index can't serve that order.
DROP INDEX IF EXISTS jobs_posted_at_idx;
