package scraper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"job-scraper-go/internal/job"
)

// Result is what one scraper produced in one run.
type Result struct {
	Source   string
	Jobs     []job.Job
	Err      error
	Duration time.Duration
}

type RunOptions struct {
	// MaxConcurrent caps how many scrapers run at once. With hundreds of
	// companies, starting them all together would open hundreds of
	// connections; this keeps the load bounded no matter the list size.
	MaxConcurrent int
	// Timeout bounds each scraper, so one slow site can't stall the run.
	Timeout time.Duration
}

// RunAll runs the scrapers concurrently and waits for all of them. Results
// come back in the same order as the scrapers.
//
// Design notes:
//   - Scrapers are I/O bound (waiting on the network), so running them in
//     parallel makes a run take about as long as the slowest few sources
//     instead of the sum of all of them.
//   - A buffered channel acts as a semaphore: a goroutine must put a token
//     in before scraping and takes it out when done, so at most
//     MaxConcurrent run at any moment.
//   - A failing scraper doesn't cancel the others. Its error is returned in
//     its Result, and the caller decides what to do (we log and move on).
//   - Each goroutine writes only to its own slot in the results slice, so no
//     mutex is needed. The WaitGroup just tells us when all are done.
func RunAll(ctx context.Context, scrapers []Scraper, opts RunOptions) []Result {
	results := make([]Result, len(scrapers))
	sem := make(chan struct{}, max(opts.MaxConcurrent, 1))
	var wg sync.WaitGroup

	for i, s := range scrapers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}: // acquired a slot
			case <-ctx.Done():
				results[i] = Result{Source: s.Name(), Err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			results[i] = runOne(ctx, s, opts.Timeout)
		}()
	}

	wg.Wait()
	return results
}

func runOne(ctx context.Context, s Scraper, timeout time.Duration) (res Result) {
	res.Source = s.Name()
	start := time.Now()

	// A bug in one scraper (nil pointer, bad index) must not crash the whole
	// process, so turn panics into ordinary errors.
	defer func() {
		if r := recover(); r != nil {
			res.Err = fmt.Errorf("scraper panicked: %v", r)
		}
		res.Duration = time.Since(start)
	}()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res.Jobs, res.Err = s.Scrape(ctx)
	return res
}
