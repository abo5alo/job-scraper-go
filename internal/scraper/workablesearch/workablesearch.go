// Package workablesearch reads Workable's public job search, the API behind
// jobs.workable.com, one country at a time.
//
// Every company that hires through Workable has its jobs listed there, so a
// single country search covers thousands of companies we'd never find by
// hand. The ats package's per-company Workable feed is the documented
// alternative; this search isn't officially documented, which is why the
// scraper checks its own results carefully (see Scrape).
package workablesearch

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/scraper"
)

const DefaultURL = "https://jobs.workable.com/api/v1/jobs"

const (
	// maxPages stops a runaway loop if the API ever keeps returning a next
	// page token forever. 1000 pages is 20,000 jobs, far more than any
	// country has today.
	maxPages = 1000
	// minCompleteness is how much of the advertised total we must collect
	// to trust the run. See Scrape.
	minCompleteness = 0.9
)

// Scraper searches one country.
type Scraper struct {
	client  *scraper.Client
	url     string
	country string // name as Workable expects it, e.g. "Saudi Arabia"
	code    string // our ISO code for it, e.g. "SA"
}

func New(client *scraper.Client, baseURL, country string) (*Scraper, error) {
	code := job.CountryCode(country)
	if code == "" {
		return nil, fmt.Errorf("workable search: unknown country %q", country)
	}
	return &Scraper{client: client, url: baseURL, country: country, code: code}, nil
}

func (s *Scraper) Name() string { return "workable-search/" + strings.ToLower(s.code) }

type searchResponse struct {
	TotalSize     int         `json:"totalSize"`
	NextPageToken string      `json:"nextPageToken"`
	Jobs          []searchJob `json:"jobs"`
}

type searchJob struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	URL                 string `json:"url"`
	Description         string `json:"description"`
	RequirementsSection string `json:"requirementsSection"`
	Workplace           string `json:"workplace"` // "on_site", "hybrid", "remote"
	Created             string `json:"created"`
	Location            struct {
		City        string `json:"city"`
		CountryName string `json:"countryName"`
	} `json:"location"`
	Company struct {
		Title string `json:"title"`
	} `json:"company"`
}

// Scrape pages through every result for the country.
//
// This source can close thousands of jobs in one run (its results are a
// full listing, so anything missing is treated as closed). That makes a
// silently partial run dangerous: if the API changed and pagination stopped
// after page 1, we'd close every other job in the country. So the scraper
// compares what it collected with the total the API reported, and fails the
// run instead of returning a result it can't trust.
func (s *Scraper) Scrape(ctx context.Context) ([]job.Job, error) {
	var (
		jobs      []job.Job
		collected int
		total     int
		token     string
	)

	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("stopped after %d pages; the API may be looping", maxPages)
		}

		q := url.Values{"location": {s.country}}
		if token != "" {
			q.Set("pageToken", token)
		}

		var resp searchResponse
		if err := s.client.GetJSON(ctx, s.url+"?"+q.Encode(), &resp); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if page == 1 {
			total = resp.TotalSize
		}
		collected += len(resp.Jobs)

		for _, r := range resp.Jobs {
			// The search matches location text loosely, so double-check
			// the job really is in the country we asked for.
			if job.CountryCode(r.Location.CountryName) != s.code {
				continue
			}
			jobs = append(jobs, s.normalize(r))
		}

		if resp.NextPageToken == "" || len(resp.Jobs) == 0 {
			break
		}
		token = resp.NextPageToken
	}

	if total > 0 && float64(collected) < minCompleteness*float64(total) {
		return nil, fmt.Errorf("collected %d of %d advertised jobs; refusing a partial result", collected, total)
	}
	return jobs, nil
}

func (s *Scraper) normalize(r searchJob) job.Job {
	return job.Job{
		Source:      "workable",
		ExternalID:  "search:" + r.ID,
		Title:       scraper.CleanText(r.Title),
		Company:     scraper.CleanText(r.Company.Title),
		Location:    joinNonEmpty(r.Location.City, r.Location.CountryName),
		Country:     s.code,
		Remote:      r.Workplace == "remote",
		URL:         r.URL,
		Description: scraper.HTMLToText(r.Description + " " + r.RequirementsSection),
		PostedAt:    parseTime(r.Created),
	}
}

// LoadCountries reads the workable_search section of the sources file.
func LoadCountries(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		WorkableSearch struct {
			Countries []string `yaml:"countries"`
		} `yaml:"workable_search"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return file.WorkableSearch.Countries, nil
}

func parseTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}
