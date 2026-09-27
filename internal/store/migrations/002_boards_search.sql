-- board: the feed a job came from (e.g. "greenhouse/careem"). A board's feed
-- lists every open job, so a job missing from it has closed.
ALTER TABLE jobs ADD COLUMN board TEXT NOT NULL DEFAULT '';
-- country: ISO 3166-1 alpha-2 code ("AE", "SA"), '' when unknown.
ALTER TABLE jobs ADD COLUMN country TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN seniority TEXT NOT NULL DEFAULT 'mid';
-- closed_at: set when a job disappears from its board's feed.
ALTER TABLE jobs ADD COLUMN closed_at TIMESTAMPTZ;

-- Full-text search document, kept up to date by Postgres on every write.
-- Weights make a match in the title (A) rank above the company (B), which
-- ranks above the description (C). The 'english' config handles word forms,
-- so "engineer" also matches "engineering" and "engineers".
ALTER TABLE jobs ADD COLUMN search tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('english'::regconfig, title), 'A') ||
    setweight(to_tsvector('english'::regconfig, company), 'B') ||
    setweight(to_tsvector('english'::regconfig, left(description, 20000)), 'C')
) STORED;

CREATE INDEX jobs_search_idx ON jobs USING GIN (search);
CREATE INDEX jobs_board_idx  ON jobs (board);
-- Partial index: search almost always filters to open jobs, so only index those.
CREATE INDEX jobs_open_country_seniority_idx ON jobs (country, seniority) WHERE closed_at IS NULL;

-- Every existing row came from Remote OK, before boards existed.
UPDATE jobs SET board = 'remoteok' WHERE board = '';
