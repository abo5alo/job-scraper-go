CREATE TABLE jobs (
    id              BIGSERIAL PRIMARY KEY,
    source          TEXT        NOT NULL,
    external_id     TEXT        NOT NULL,
    title           TEXT        NOT NULL,
    company         TEXT        NOT NULL,
    location        TEXT        NOT NULL DEFAULT '',
    remote          BOOLEAN     NOT NULL DEFAULT false,
    url             TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    tags            TEXT[]      NOT NULL DEFAULT '{}',
    salary_min      INTEGER,
    salary_max      INTEGER,
    salary_currency TEXT        NOT NULL DEFAULT '',
    posted_at       TIMESTAMPTZ,
    -- first_seen_at / last_seen_at let us tell active postings from stale
    -- ones: a job that stops appearing in scrapes is probably filled.
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (source, external_id)
);

CREATE INDEX jobs_posted_at_idx    ON jobs (posted_at DESC);
CREATE INDEX jobs_last_seen_at_idx ON jobs (last_seen_at DESC);
-- GIN index makes "jobs tagged golang" (tags @> '{golang}') fast.
CREATE INDEX jobs_tags_idx         ON jobs USING GIN (tags);
