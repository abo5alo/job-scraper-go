// Command scraper runs every job source once and saves the results.
//
//	go run ./cmd/scraper
//	go run ./cmd/scraper -sources path/to/sources.yaml
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"job-scraper-go/internal/scraper"
	"job-scraper-go/internal/scraper/ats"
	"job-scraper-go/internal/scraper/remoteok"
	"job-scraper-go/internal/scraper/workablesearch"
	"job-scraper-go/internal/store"
)

func main() {
	sourcesPath := flag.String("sources", "sources.yaml", "file listing the job sources to scrape")
	flag.Parse()

	// slog gives structured key=value logs, which are much easier to search
	// than free-form Printf output once the app runs in production.
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := run(log, *sourcesPath); err != nil {
		log.Error("scraper failed", "err", err)
		os.Exit(1)
	}
}

// source pairs a scraper with whether its feed lists every open job.
type source struct {
	scraper scraper.Scraper
	// fullListing is true for company ATS boards and Workable country
	// searches: a job missing from the feed has closed. It's false for
	// Remote OK, whose API only returns its latest ~100 postings, so a job
	// falling off that list means nothing.
	fullListing bool
}

func run(log *slog.Logger, sourcesPath string) error {
	// Ctrl+C cancels ctx, which cancels in-flight HTTP requests and queries.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	boards, err := ats.LoadBoards(sourcesPath)
	if err != nil {
		return err
	}
	countries, err := workablesearch.LoadCountries(sourcesPath)
	if err != nil {
		return err
	}

	db, err := store.New(ctx, envOr("DATABASE_URL", "postgres://jobs:jobs@localhost:5432/jobs?sslmode=disable"))
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}

	// One shared client, so rate limits apply per host across all scrapers:
	// Careem and Tamara are both on Greenhouse and share its limit.
	client := scraper.NewClient(scraper.DefaultClientOptions())

	sources := []source{{remoteok.New(client, remoteok.DefaultURL), false}}
	for _, b := range boards {
		s, err := ats.New(b, client)
		if err != nil {
			return err
		}
		sources = append(sources, source{s, true})
	}
	for _, c := range countries {
		s, err := workablesearch.New(client, workablesearch.DefaultURL, c)
		if err != nil {
			return err
		}
		sources = append(sources, source{s, true})
	}

	scrapers := make([]scraper.Scraper, len(sources))
	configured := make([]string, len(sources))
	for i, s := range sources {
		scrapers[i] = s.scraper
		configured[i] = s.scraper.Name()
	}

	seenAt := time.Now()

	// Jobs from sources removed from sources.yaml would otherwise stay open
	// forever, because nothing scrapes their board anymore.
	if n, err := db.CloseRemovedBoards(ctx, configured, seenAt); err != nil {
		return err
	} else if n > 0 {
		log.Info("closed jobs from removed sources", "jobs", n)
	}

	// The timeout is generous because the Workable country searches share
	// one host, and so one rate limit: together they need ~450 requests,
	// which at 2 per second is almost 4 minutes, and that's by design.
	results := scraper.RunAll(ctx, scrapers, scraper.RunOptions{MaxConcurrent: 8, Timeout: 10 * time.Minute})

	var totalJobs, failed int
	for i, r := range results {
		if r.Err != nil {
			failed++
			log.Error("scrape failed", "source", r.Source, "duration", r.Duration.Round(time.Millisecond), "err", r.Err)
			continue
		}

		for j := range r.Jobs {
			r.Jobs[j].Board = r.Source
			r.Jobs[j].Normalize()
		}
		if err := db.UpsertJobs(ctx, r.Jobs, seenAt); err != nil {
			failed++
			log.Error("save failed", "source", r.Source, "err", err)
			continue
		}
		totalJobs += len(r.Jobs)

		var closed int64
		if sources[i].fullListing {
			if len(r.Jobs) == 0 {
				// An empty feed usually means the site changed or broke, not
				// that the company filled every role at once. Closing
				// everything would wipe the board on a glitch.
				log.Warn("board returned no jobs; leaving its jobs open", "source", r.Source)
			} else if closed, err = db.CloseMissing(ctx, r.Source, seenAt); err != nil {
				log.Error("close failed", "source", r.Source, "err", err)
			}
		}

		log.Info("scraped", "source", r.Source, "jobs", len(r.Jobs), "closed", closed,
			"duration", r.Duration.Round(time.Millisecond))
	}

	log.Info("run finished", "sources", len(results), "failed", failed, "jobs", totalJobs,
		"duration", time.Since(seenAt).Round(time.Millisecond))

	if failed == len(results) {
		return errors.New("every source failed")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
