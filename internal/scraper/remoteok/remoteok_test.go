package remoteok

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"job-scraper-go/internal/scraper"
)

// A trimmed copy of the real API response: legal notice first, then jobs.
const sampleResponse = `[
	{"last_updated": 1790438426, "legal": "API Terms of Service..."},
	{
		"id": "1137431",
		"epoch": 1790265606,
		"company": " Acme Corp ",
		"position": "Senior Go Engineer",
		"tags": ["Golang", "backend", "golang", ""],
		"description": "<p>Build things.</p><ul><li>Go</li><li>Postgres</li></ul>",
		"location": "Worldwide",
		"url": "https://remoteOK.com/remote-jobs/1137431",
		"salary_min": 120000,
		"salary_max": 0
	}
]`

func TestScrape(t *testing.T) {
	// httptest.NewServer runs a real HTTP server on localhost, so the test
	// exercises our actual HTTP + JSON code without touching the internet.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request sent without a User-Agent")
		}
		w.Write([]byte(sampleResponse))
	}))
	defer srv.Close()

	jobs, err := New(testClient(), srv.URL).Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (legal notice should be skipped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "1137431" || j.Source != "remoteok" {
		t.Errorf("identity = %s/%s", j.Source, j.ExternalID)
	}
	if j.Company != "Acme Corp" {
		t.Errorf("Company = %q, want trimmed", j.Company)
	}
	if j.Description != "Build things. Go Postgres" {
		t.Errorf("Description = %q", j.Description)
	}
	if got := j.Tags; len(got) != 2 || got[0] != "golang" || got[1] != "backend" {
		t.Errorf("Tags = %v, want [golang backend]", got)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 120000 {
		t.Errorf("SalaryMin = %v, want 120000", j.SalaryMin)
	}
	if j.SalaryMax != nil {
		t.Errorf("SalaryMax = %v, want nil (0 means unknown)", *j.SalaryMax)
	}
	if j.PostedAt == nil || j.PostedAt.Unix() != 1790265606 {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

func TestScrapeBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	if _, err := New(testClient(), srv.URL).Scrape(context.Background()); err == nil {
		t.Fatal("expected an error for a 429 response")
	}
}

func testClient() *scraper.Client {
	return scraper.NewClient(scraper.ClientOptions{RequestsPerSecond: 1000, Timeout: 5 * time.Second})
}
