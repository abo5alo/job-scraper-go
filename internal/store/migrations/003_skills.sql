-- skills: names detected by job.SkillsFromText, e.g. {Python,SQL,Arabic}.
-- Existing rows get them on the next scrape, or right away with
-- "go run ./cmd/scraper -renormalize".
ALTER TABLE jobs ADD COLUMN skills TEXT[] NOT NULL DEFAULT '{}';

-- GIN makes "jobs that need Python" (skills @> '{Python}') fast.
CREATE INDEX jobs_skills_idx ON jobs USING GIN (skills);
