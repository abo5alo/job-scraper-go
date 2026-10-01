// Package scraper defines what a job source looks like and runs many of them
// concurrently. Each job board lives in its own subpackage and implements
// the Scraper interface.
package scraper

import (
	"context"

	"job-scraper-go/internal/job"
)

// Scraper is the one interface every source implements. It is intentionally
// small: the runner only needs a name for logging and a way to fetch jobs.
type Scraper interface {
	// Name identifies the feed, e.g. "remoteok" or "greenhouse/careem".
	Name() string
	Scrape(ctx context.Context) ([]job.Job, error)
}

// UserAgent identifies us honestly to the sites we fetch from.
const UserAgent = "job-scraper-go/0.1 (learning project)"
