package store

import (
	"context"
	"os"
	"testing"
	"time"

	"job-scraper-go/internal/job"
)

// These are integration tests: they run real SQL against a real Postgres,
// because full-text search, ON CONFLICT and generated columns can't be
// faked meaningfully. They use a separate database, since they wipe the
// jobs table, and are skipped unless TEST_DATABASE_URL is set:
//
//	docker compose exec postgres createdb -U jobs jobs_test
//	TEST_DATABASE_URL=postgres://jobs:jobs@localhost:5432/jobs_test?sslmode=disable go test ./internal/store
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	s, err := New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)

	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, "TRUNCATE jobs"); err != nil {
		t.Fatal(err)
	}
	return s
}

func testJob(id, title, country string, level job.Seniority) job.Job {
	return job.Job{
		Source: "greenhouse", ExternalID: id, Board: "greenhouse/acme",
		Title: title, Company: "Acme", Country: country, Seniority: level,
		URL: "https://example.com/" + id, Description: "Work on payments systems.",
	}
}

func TestSearchAndClose(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	run1 := time.Now().Add(-time.Hour)
	err := s.UpsertJobs(ctx, []job.Job{
		testJob("1", "Senior Backend Engineer", "AE", job.SenioritySenior),
		testJob("2", "Junior Frontend Developer", "SA", job.SeniorityJunior),
		testJob("3", "Accountant", "AE", job.SeniorityMid),
	}, run1)
	if err != nil {
		t.Fatal(err)
	}

	search := func(p SearchParams) SearchResult {
		t.Helper()
		if p.Limit == 0 {
			p.Limit = 10
		}
		res, err := s.SearchJobs(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Stemming: "engineering" finds "Engineer".
	if res := search(SearchParams{Query: "engineering", Sort: SortRelevance}); res.Total != 1 || res.Jobs[0].ExternalID != "1" {
		t.Errorf("q=engineering: %+v", res)
	}
	// Filters combine with AND.
	if res := search(SearchParams{Countries: []string{"AE"}, Seniority: []string{"mid"}}); res.Total != 1 || res.Jobs[0].Title != "Accountant" {
		t.Errorf("country=AE level=mid: %+v", res)
	}
	// Description text is searchable too.
	if res := search(SearchParams{Query: "payments"}); res.Total != 3 {
		t.Errorf("q=payments: total = %d, want 3", res.Total)
	}
	// Paging: total counts every match, the page holds only limit rows.
	if res := search(SearchParams{Limit: 2}); res.Total != 3 || len(res.Jobs) != 2 {
		t.Errorf("limit=2: total=%d len=%d", res.Total, len(res.Jobs))
	}

	// Second run sees only job 1: jobs 2 and 3 should close.
	run2 := time.Now()
	if err := s.UpsertJobs(ctx, []job.Job{testJob("1", "Senior Backend Engineer", "AE", job.SenioritySenior)}, run2); err != nil {
		t.Fatal(err)
	}
	closed, err := s.CloseMissing(ctx, "greenhouse/acme", run2)
	if err != nil || closed != 2 {
		t.Fatalf("CloseMissing = %d, %v; want 2", closed, err)
	}
	if res := search(SearchParams{}); res.Total != 1 {
		t.Errorf("open jobs = %d, want 1", res.Total)
	}
	if res := search(SearchParams{IncludeClosed: true}); res.Total != 3 {
		t.Errorf("all jobs = %d, want 3", res.Total)
	}

	// A closed job that reappears is reopened, keeping its first_seen_at.
	run3 := time.Now().Add(time.Minute)
	if err := s.UpsertJobs(ctx, []job.Job{testJob("3", "Accountant", "AE", job.SeniorityMid)}, run3); err != nil {
		t.Fatal(err)
	}
	res := search(SearchParams{Query: "accountant"})
	if res.Total != 1 || res.Jobs[0].ClosedAt != nil || !res.Jobs[0].FirstSeenAt.Before(res.Jobs[0].LastSeenAt) {
		t.Errorf("reopened job: %+v", res.Jobs)
	}
}

func TestCloseRemovedBoards(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	kept := testJob("1", "Engineer", "AE", job.SeniorityMid)
	removed := testJob("2", "Engineer", "SA", job.SeniorityMid)
	removed.Board = "workable/salla"
	if err := s.UpsertJobs(ctx, []job.Job{kept, removed}, time.Now()); err != nil {
		t.Fatal(err)
	}

	n, err := s.CloseRemovedBoards(ctx, []string{"greenhouse/acme"}, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("closed %d, %v; want 1 (only the unconfigured board)", n, err)
	}
	res, _ := s.SearchJobs(ctx, SearchParams{Limit: 10})
	if res.Total != 1 || res.Jobs[0].Board != "greenhouse/acme" {
		t.Errorf("open jobs = %+v", res.Jobs)
	}
}

func TestSearchHidesOldJobs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	old := time.Now().AddDate(-8, 0, 0) // like Namshi's 2017 postings
	recent := time.Now().AddDate(0, 0, -10)
	oldJob := testJob("1", "Stylist", "AE", job.SeniorityMid)
	oldJob.PostedAt = &old
	newJob := testJob("2", "Stylist", "AE", job.SeniorityMid)
	newJob.PostedAt = &recent
	undated := testJob("3", "Stylist", "AE", job.SeniorityMid) // falls back to first_seen_at (now)

	if err := s.UpsertJobs(ctx, []job.Job{oldJob, newJob, undated}, time.Now()); err != nil {
		t.Fatal(err)
	}

	since := time.Now().AddDate(0, -3, 0)
	res, err := s.SearchJobs(ctx, SearchParams{PostedSince: since, Limit: 10})
	if err != nil || res.Total != 2 {
		t.Fatalf("total = %d, err = %v; want 2 (the 8-year-old job hidden)", res.Total, err)
	}
	counts, err := s.CountryCounts(ctx, since)
	if err != nil || len(counts) != 1 || counts[0].Count != 2 {
		t.Errorf("CountryCounts = %+v, %v; want AE: 2", counts, err)
	}
}

func TestSearchEscapesLikeWildcards(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	j := testJob("1", "Engineer", "AE", job.SeniorityMid)
	j.Company = "Acme"
	if err := s.UpsertJobs(ctx, []job.Job{j}, time.Now()); err != nil {
		t.Fatal(err)
	}

	// "%" must match a literal percent sign, not act as "anything".
	res, err := s.SearchJobs(ctx, SearchParams{Company: "%", Limit: 10})
	if err != nil || res.Total != 0 {
		t.Errorf("company=%%: total=%d err=%v, want 0", res.Total, err)
	}
}
