package ats

import (
	"context"
	"strconv"
	"strings"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/scraper"
)

// recruitee reads https://{slug}.recruitee.com/api/offers/
type recruitee struct{ base }

type recruiteeResponse struct {
	Offers []struct {
		ID             int64  `json:"id"`
		Title          string `json:"title"`
		CareersURL     string `json:"careers_url"`
		Location       string `json:"location"`
		CountryCode    string `json:"country_code"`
		Remote         bool   `json:"remote"`
		Description    string `json:"description"`
		Requirements   string `json:"requirements"`
		PublishedAt    string `json:"published_at"` // "2026-09-10 07:25:25 UTC"
		ExperienceCode string `json:"experience_code"`
		Salary         struct {
			Min      flexNumber `json:"min"`
			Max      flexNumber `json:"max"`
			Period   string     `json:"period"`
			Currency string     `json:"currency"`
		} `json:"salary"`
	} `json:"offers"`
}

func (r *recruitee) Scrape(ctx context.Context) ([]job.Job, error) {
	var resp recruiteeResponse
	if err := r.client.GetJSON(ctx, r.baseURL+"/api/offers/", &resp); err != nil {
		return nil, err
	}

	jobs := make([]job.Job, 0, len(resp.Offers))
	for _, o := range resp.Offers {
		j := job.Job{
			Source:      "recruitee",
			ExternalID:  r.externalID(strconv.FormatInt(o.ID, 10)),
			Title:       scraper.CleanText(o.Title),
			Company:     r.board.Company,
			Location:    scraper.CleanText(o.Location),
			Country:     o.CountryCode,
			Remote:      o.Remote,
			Seniority:   job.SeniorityFromLabel(o.ExperienceCode),
			URL:         o.CareersURL,
			Description: scraper.HTMLToText(o.Description + " " + o.Requirements),
			PostedAt:    parseTime("2006-01-02 15:04:05 MST", o.PublishedAt),
		}

		// Only keep yearly salaries. Mixing monthly and yearly figures in one
		// column would make every salary statistic meaningless.
		if isYearly(o.Salary.Period) && o.Salary.Currency != "" {
			j.SalaryMin, j.SalaryMax = o.Salary.Min.value, o.Salary.Max.value
			if j.SalaryMin != nil || j.SalaryMax != nil {
				j.SalaryCurrency = strings.ToUpper(o.Salary.Currency)
			}
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func isYearly(period string) bool {
	switch strings.ToLower(period) {
	case "year", "yearly", "annual", "annually":
		return true
	}
	return false
}

// flexNumber decodes a JSON number that may arrive as a number (5000), a
// string ("5000"), or null. Unparseable values become "unknown" instead of
// failing the whole scrape over one odd salary field.
type flexNumber struct{ value *int }

func (n *flexNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		v := int(f)
		n.value = &v
	}
	return nil
}
