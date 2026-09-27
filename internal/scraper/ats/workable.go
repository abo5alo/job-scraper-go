package ats

import (
	"context"
	"fmt"
	"time"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/scraper"
)

// workable reads https://apply.workable.com/api/v1/widget/accounts/{slug}
type workable struct{ base }

type workableResponse struct {
	Jobs []struct {
		Title         string `json:"title"`
		Shortcode     string `json:"shortcode"`
		URL           string `json:"url"`
		PublishedOn   string `json:"published_on"` // "2026-09-21"
		City          string `json:"city"`
		Country       string `json:"country"`
		Telecommuting bool   `json:"telecommuting"`
		Experience    string `json:"experience"` // "Entry level", "Mid-Senior level", ...
		Description   string `json:"description"`
		Locations     []struct {
			CountryCode string `json:"countryCode"`
		} `json:"locations"`
	} `json:"jobs"`
}

func (w *workable) Scrape(ctx context.Context) ([]job.Job, error) {
	var resp workableResponse
	url := fmt.Sprintf("%s/api/v1/widget/accounts/%s?details=true", w.baseURL, w.board.Slug)
	if err := w.client.GetJSON(ctx, url, &resp); err != nil {
		return nil, err
	}

	jobs := make([]job.Job, 0, len(resp.Jobs))
	for _, r := range resp.Jobs {
		country := r.Country
		if len(r.Locations) > 0 && r.Locations[0].CountryCode != "" {
			country = r.Locations[0].CountryCode
		}

		jobs = append(jobs, job.Job{
			Source:      "workable",
			ExternalID:  w.externalID(r.Shortcode),
			Title:       scraper.CleanText(r.Title),
			Company:     w.board.Company,
			Location:    joinNonEmpty(r.City, r.Country),
			Country:     country,
			Remote:      r.Telecommuting,
			Seniority:   job.SeniorityFromLabel(r.Experience),
			URL:         r.URL,
			Description: scraper.HTMLToText(r.Description),
			PostedAt:    parseTime(time.DateOnly, r.PublishedOn),
		})
	}
	return jobs, nil
}
