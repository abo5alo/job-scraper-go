package workablesearch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"job-scraper-go/internal/scraper"
)

func testClient() *scraper.Client {
	return scraper.NewClient(scraper.ClientOptions{RequestsPerSecond: 1000, Timeout: 5 * time.Second})
}

// jobJSON is one search result, trimmed from a real response.
func jobJSON(id, country string) string {
	return fmt.Sprintf(`{
		"id": %q, "title": "Receptionist &amp; Admin",
		"url": "https://jobs.workable.com/view/%s/receptionist",
		"description": "<p>Greet guests.</p>", "requirementsSection": "<ul><li>English</li></ul>",
		"workplace": "remote", "created": "2026-09-24T14:30:45.239Z",
		"location": {"city": "Amman", "countryName": %q},
		"company": {"title": "Acme Hotels"}
	}`, id, id, country)
}

// fakeSearch serves two pages chained by a page token, like the real API.
func fakeSearch(t *testing.T, total int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("location"); got != "Jordan" {
			t.Errorf("location = %q, want Jordan", got)
		}
		switch r.URL.Query().Get("pageToken") {
		case "":
			fmt.Fprintf(w, `{"totalSize": %d, "nextPageToken": "abc", "jobs": [%s, %s]}`,
				total, jobJSON("1", "Jordan"), jobJSON("2", "Jordan"))
		case "abc":
			// Last page: no next token. Job 4 is in another country and
			// must be filtered out.
			fmt.Fprintf(w, `{"totalSize": %d, "jobs": [%s, %s]}`,
				total, jobJSON("3", "Jordan"), jobJSON("4", "Lebanon"))
		default:
			t.Errorf("unexpected page token %q", r.URL.Query().Get("pageToken"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestScrapeFollowsPagesAndFiltersCountry(t *testing.T) {
	srv := fakeSearch(t, 4)
	s, err := New(testClient(), srv.URL, "Jordan")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name() != "workable-search/jo" {
		t.Errorf("Name = %q", s.Name())
	}

	jobs, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (the Lebanon job filtered out)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "search:1" || j.Company != "Acme Hotels" || j.Country != "JO" || !j.Remote {
		t.Errorf("job = %+v", j)
	}
	if j.Title != "Receptionist & Admin" || j.Description != "Greet guests. English" {
		t.Errorf("title=%q description=%q", j.Title, j.Description)
	}
}

func TestScrapeRefusesPartialResults(t *testing.T) {
	// The API claims 100 jobs but pagination ends after 4. Returning those 4
	// would close the other 96 in the database, so it must be an error.
	srv := fakeSearch(t, 100)
	s, _ := New(testClient(), srv.URL, "Jordan")

	_, err := s.Scrape(context.Background())
	if err == nil || !strings.Contains(err.Error(), "partial") {
		t.Fatalf("err = %v, want a partial-result error", err)
	}
}

func TestNewRejectsUnknownCountry(t *testing.T) {
	if _, err := New(testClient(), DefaultURL, "Atlantis"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoadCountries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.yaml")
	os.WriteFile(path, []byte("companies: []\nworkable_search:\n  countries: [Egypt, Saudi Arabia]\n"), 0o644)

	got, err := LoadCountries(path)
	if err != nil || len(got) != 2 || got[1] != "Saudi Arabia" {
		t.Fatalf("got %v, %v", got, err)
	}
}
