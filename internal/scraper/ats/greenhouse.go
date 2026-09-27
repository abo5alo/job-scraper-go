package ats

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"time"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/scraper"
)

// greenhouse reads https://boards-api.greenhouse.io/v1/boards/{slug}/jobs
type greenhouse struct{ base }

type greenhouseResponse struct {
	Jobs []struct {
		ID             int64  `json:"id"`
		Title          string `json:"title"`
		AbsoluteURL    string `json:"absolute_url"`
		Content        string `json:"content"`
		FirstPublished string `json:"first_published"`
		UpdatedAt      string `json:"updated_at"`
		Location       struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"jobs"`
}

func (g *greenhouse) Scrape(ctx context.Context) ([]job.Job, error) {
	var resp greenhouseResponse
	url := fmt.Sprintf("%s/v1/boards/%s/jobs?content=true", g.baseURL, g.board.Slug)
	if err := g.client.GetJSON(ctx, url, &resp); err != nil {
		return nil, err
	}

	jobs := make([]job.Job, 0, len(resp.Jobs))
	for _, r := range resp.Jobs {
		jobs = append(jobs, job.Job{
			Source:     "greenhouse",
			ExternalID: g.externalID(strconv.FormatInt(r.ID, 10)),
			Title:      scraper.CleanText(r.Title),
			Company:    g.board.Company,
			Location:   scraper.CleanText(r.Location.Name),
			Remote:     isRemote(r.Location.Name),
			URL:        r.AbsoluteURL,
			// Greenhouse HTML-escapes its HTML ("&lt;p&gt;"), so it has to be
			// unescaped once before the tags can be stripped.
			Description: scraper.HTMLToText(html.UnescapeString(r.Content)),
			PostedAt:    parseTime(time.RFC3339, r.FirstPublished, r.UpdatedAt),
		})
	}
	return jobs, nil
}
