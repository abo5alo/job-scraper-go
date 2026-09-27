-- tech: whether job.IsTech classified the posting as a tech job. Non-tech
-- jobs stay stored; search can hide them with ?tech=true. Existing rows get
-- it from "go run ./cmd/scraper -renormalize" or the next scrape.
ALTER TABLE jobs ADD COLUMN tech BOOLEAN NOT NULL DEFAULT false;
