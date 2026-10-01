package ats

import (
	"context"
	"fmt"
	"strings"
	"time"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/scraper"
)

// smartRecruiters reads https://api.smartrecruiters.com/v1/companies/{slug}/postings
//
// It's the most expensive ATS to scrape: the list is paginated and has no
// descriptions, so every job needs a second request for its details. This is
// where the client's per-host rate limit earns its keep.
type smartRecruiters struct{ base }

const srPageSize = 100

type srPosting struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ReleasedDate string `json:"releasedDate"`
	Company      struct {
		Identifier string `json:"identifier"`
	} `json:"company"`
	Location struct {
		Country      string `json:"country"` // lowercase code, e.g. "ae"
		Remote       bool   `json:"remote"`
		FullLocation string `json:"fullLocation"` // "Dubai, , United Arab Emirates"
	} `json:"location"`
	ExperienceLevel struct {
		ID string `json:"id"` // "entry_level", "mid_senior_level", ...
	} `json:"experienceLevel"`
}

type srList struct {
	TotalFound int         `json:"totalFound"`
	Content    []srPosting `json:"content"`
}

type srDetail struct {
	PostingURL string `json:"postingUrl"`
	JobAd      struct {
		Sections map[string]struct {
			Text string `json:"text"`
		} `json:"sections"`
	} `json:"jobAd"`
}

func (s *smartRecruiters) Scrape(ctx context.Context) ([]job.Job, error) {
	var postings []srPosting
	for offset := 0; ; {
		var page srList
		url := fmt.Sprintf("%s/v1/companies/%s/postings?limit=%d&offset=%d", s.baseURL, s.board.Slug, srPageSize, offset)
		if err := s.client.GetJSON(ctx, url, &page); err != nil {
			return nil, err
		}
		postings = append(postings, page.Content...)
		offset += len(page.Content)
		if len(page.Content) == 0 || offset >= page.TotalFound {
			break
		}
	}

	jobs := make([]job.Job, 0, len(postings))
	for _, p := range postings {
		j := job.Job{
			Source:     "smartrecruiters",
			ExternalID: s.externalID(p.ID),
			Title:      scraper.CleanText(p.Name),
			Company:    s.board.Company,
			Location:   scraper.JoinNonEmpty(strings.Split(p.Location.FullLocation, ",")...),
			Country:    p.Location.Country,
			Remote:     p.Location.Remote,
			Seniority:  job.SeniorityFromLabel(p.ExperienceLevel.ID),
			// Fallback link in case the detail request below fails.
			URL:      fmt.Sprintf("https://jobs.smartrecruiters.com/%s/%s", p.Company.Identifier, p.ID),
			PostedAt: scraper.ParseTime(time.RFC3339, p.ReleasedDate),
		}

		var d srDetail
		url := fmt.Sprintf("%s/v1/companies/%s/postings/%s", s.baseURL, s.board.Slug, p.ID)
		err := s.client.GetJSON(ctx, url, &d)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// If the detail request fails (say the job closed between the two
		// requests), keep the job without a description rather than failing
		// the whole company. It will be closed on the next run if it's gone.
		if err == nil {
			if d.PostingURL != "" {
				j.URL = d.PostingURL
			}
			j.Description = srDescription(d)
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// srDescription joins the job ad's sections in reading order. The company
// description section is skipped: it's identical boilerplate on every job,
// and it would make every job match searches for the company's industry.
func srDescription(d srDetail) string {
	var parts []string
	for _, key := range []string{"jobDescription", "qualifications", "additionalInformation"} {
		if text := scraper.HTMLToText(d.JobAd.Sections[key].Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}
