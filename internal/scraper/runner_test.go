package scraper

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"job-scraper-go/internal/job"
)

// fakeScraper lets us test the runner without any network.
type fakeScraper struct {
	name  string
	delay time.Duration
	jobs  []job.Job
	err   error
	panic bool
}

func (f fakeScraper) Name() string { return f.name }

func (f fakeScraper) Scrape(ctx context.Context) ([]job.Job, error) {
	if f.panic {
		panic("boom")
	}
	select {
	case <-time.After(f.delay):
		return f.jobs, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestRunAll(t *testing.T) {
	scrapers := []Scraper{
		fakeScraper{name: "ok", delay: 50 * time.Millisecond, jobs: []job.Job{{Title: "a"}}},
		fakeScraper{name: "fails", err: errors.New("site down")},
		fakeScraper{name: "slow", delay: time.Hour},
		fakeScraper{name: "panics", panic: true},
	}

	start := time.Now()
	results := RunAll(context.Background(), scrapers, RunOptions{MaxConcurrent: 4, Timeout: 200 * time.Millisecond})

	// If scrapers ran one after another this would take far longer, and
	// "slow" would never finish. The timeout keeps the run bounded.
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("RunAll took %v; scrapers should run concurrently with a timeout", elapsed)
	}

	if len(results[0].Jobs) != 1 || results[0].Err != nil {
		t.Errorf("ok: %+v", results[0])
	}
	if results[1].Err == nil {
		t.Error("fails: expected error")
	}
	if !errors.Is(results[2].Err, context.DeadlineExceeded) {
		t.Errorf("slow: err = %v, want deadline exceeded", results[2].Err)
	}
	if results[3].Err == nil {
		t.Error("panics: expected panic to be turned into an error")
	}
}

// countingScraper records how many scrapers are running at the same moment.
type countingScraper struct {
	running, peak *atomic.Int32
}

func (c countingScraper) Name() string { return "counting" }

func (c countingScraper) Scrape(ctx context.Context) ([]job.Job, error) {
	n := c.running.Add(1)
	defer c.running.Add(-1)
	for {
		p := c.peak.Load()
		if n <= p || c.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	return nil, nil
}

func TestRunAllRespectsMaxConcurrent(t *testing.T) {
	var running, peak atomic.Int32
	scrapers := make([]Scraper, 20)
	for i := range scrapers {
		scrapers[i] = countingScraper{&running, &peak}
	}

	RunAll(context.Background(), scrapers, RunOptions{MaxConcurrent: 3, Timeout: time.Second})

	if got := peak.Load(); got != 3 {
		t.Errorf("peak concurrency = %d, want 3", got)
	}
}

func ExampleRunAll() {
	scrapers := []Scraper{
		fakeScraper{name: "a", jobs: []job.Job{{Title: "Go Engineer"}}},
		fakeScraper{name: "b", err: errors.New("site down")},
	}
	for _, r := range RunAll(context.Background(), scrapers, RunOptions{MaxConcurrent: 2, Timeout: time.Second}) {
		fmt.Println(r.Source, len(r.Jobs), r.Err)
	}
	// Output:
	// a 1 <nil>
	// b 0 site down
}
