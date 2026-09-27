package ats

import (
	"context"
	"fmt"
	"strings"
	"time"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/scraper"
)

// ashby reads https://api.ashbyhq.com/posting-api/job-board/{slug}
type ashby struct{ base }

type ashbyResponse struct {
	Jobs []struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		Location         string `json:"location"`
		IsListed         bool   `json:"isListed"`
		IsRemote         bool   `json:"isRemote"`
		WorkplaceType    string `json:"workplaceType"` // "OnSite", "Hybrid", "Remote" or empty
		PublishedAt      string `json:"publishedAt"`
		JobURL           string `json:"jobUrl"`
		DescriptionPlain string `json:"descriptionPlain"`
		Address          *struct {
			PostalAddress struct {
				AddressCountry string `json:"addressCountry"`
			} `json:"postalAddress"`
		} `json:"address"`
	} `json:"jobs"`
}

func (a *ashby) Scrape(ctx context.Context) ([]job.Job, error) {
	var resp ashbyResponse
	url := fmt.Sprintf("%s/posting-api/job-board/%s", a.baseURL, a.board.Slug)
	if err := a.client.GetJSON(ctx, url, &resp); err != nil {
		return nil, err
	}

	jobs := make([]job.Job, 0, len(resp.Jobs))
	for _, r := range resp.Jobs {
		if !r.IsListed {
			continue // unlisted jobs are hidden from the public careers page
		}

		// Ashby sets isRemote for hybrid jobs too, so prefer workplaceType.
		remote := r.WorkplaceType == "Remote" || (r.WorkplaceType == "" && r.IsRemote)

		j := job.Job{
			Source:      "ashby",
			ExternalID:  a.externalID(r.ID),
			Title:       scraper.CleanText(r.Title),
			Company:     a.board.Company,
			Location:    scraper.CleanText(r.Location),
			Remote:      remote,
			URL:         r.JobURL,
			Description: strings.Join(strings.Fields(r.DescriptionPlain), " "),
			PostedAt:    parseTime(time.RFC3339, r.PublishedAt),
		}
		// Location is free text like "Cairo Office"; the address, when set,
		// has the country as a proper field.
		if r.Address != nil {
			j.Country = r.Address.PostalAddress.AddressCountry
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}
