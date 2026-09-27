// Package remoteok ingests jobs from the Remote OK public JSON API.
//
// Remote OK's API terms ask that we credit them and link back to the
// original posting, so we always keep their URL as the job's URL.
package remoteok

import (
	"context"
	"strings"
	"time"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/scraper"
)

const DefaultURL = "https://remoteok.com/api"

type Scraper struct {
	client *scraper.Client
	url    string
}

// New takes the URL as a parameter so tests can point the scraper at a local
// fake server instead of the real site.
func New(client *scraper.Client, url string) *Scraper {
	return &Scraper{client: client, url: url}
}

func (s *Scraper) Name() string { return "remoteok" }

// apiJob mirrors the JSON exactly. We decode into this first, then convert to
// job.Job, which keeps the source's quirks out of our unified schema.
type apiJob struct {
	ID          string   `json:"id"`
	Epoch       int64    `json:"epoch"`
	Company     string   `json:"company"`
	Position    string   `json:"position"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	URL         string   `json:"url"`
	SalaryMin   int      `json:"salary_min"`
	SalaryMax   int      `json:"salary_max"`
}

func (s *Scraper) Scrape(ctx context.Context) ([]job.Job, error) {
	var raw []apiJob
	if err := s.client.GetJSON(ctx, s.url, &raw); err != nil {
		return nil, err
	}

	jobs := make([]job.Job, 0, len(raw))
	for _, r := range raw {
		// The first array element is a legal notice, not a job. It has no ID.
		if r.ID == "" {
			continue
		}
		jobs = append(jobs, normalize(r))
	}
	return jobs, nil
}

func normalize(r apiJob) job.Job {
	j := job.Job{
		Source:      "remoteok",
		ExternalID:  r.ID,
		Title:       scraper.CleanText(r.Position),
		Company:     scraper.CleanText(r.Company),
		Location:    scraper.CleanText(r.Location),
		Remote:      true, // every Remote OK listing is remote
		URL:         r.URL,
		Description: scraper.FixMojibake(scraper.HTMLToText(r.Description)),
		Tags:        normalizeTags(r.Tags),
	}

	// Remote OK uses 0 for "not listed". Store unknown as NULL, not 0.
	if r.SalaryMin > 0 {
		j.SalaryMin = &r.SalaryMin
		j.SalaryCurrency = "USD"
	}
	if r.SalaryMax > 0 {
		j.SalaryMax = &r.SalaryMax
		j.SalaryCurrency = "USD"
	}

	if r.Epoch > 0 {
		t := time.Unix(r.Epoch, 0).UTC()
		j.PostedAt = &t
	}
	return j
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, t := range tags {
		t = strings.ToLower(scraper.CleanText(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
