package api

import (
	"time"

	"job-scraper-go/internal/store"
)

// The JSON shapes are separate types from job.Job on purpose: the API is a
// contract with clients, and it shouldn't change every time an internal
// struct gets a new field.

type searchResponse struct {
	Total int           `json:"total"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
	Jobs  []jobResponse `json:"jobs"`
}

type jobResponse struct {
	ID          int64           `json:"id"`
	Title       string          `json:"title"`
	Company     string          `json:"company"`
	Location    string          `json:"location,omitempty"`
	Country     string          `json:"country,omitempty"`
	Remote      bool            `json:"remote"`
	Level       string          `json:"level"`
	URL         string          `json:"url"`
	Source      string          `json:"source"`
	Salary      *salaryResponse `json:"salary,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Description string          `json:"description,omitempty"`
	PostedAt    *time.Time      `json:"posted_at,omitempty"`
	FirstSeenAt time.Time       `json:"first_seen_at"`
	ClosedAt    *time.Time      `json:"closed_at,omitempty"`
}

type salaryResponse struct {
	Min      *int   `json:"min,omitempty"`
	Max      *int   `json:"max,omitempty"`
	Currency string `json:"currency"`
}

// Search results show a preview; GET /jobs/{id} has the full description.
const listDescriptionLen = 300

// toJobResponse converts a stored job. maxDesc > 0 truncates the description.
func toJobResponse(j store.JobRecord, maxDesc int) jobResponse {
	r := jobResponse{
		ID:          j.ID,
		Title:       j.Title,
		Company:     j.Company,
		Location:    j.Location,
		Country:     j.Country,
		Remote:      j.Remote,
		Level:       string(j.Seniority),
		URL:         j.URL,
		Source:      j.Source,
		Tags:        j.Tags,
		Description: j.Description,
		PostedAt:    j.PostedAt,
		FirstSeenAt: j.FirstSeenAt,
		ClosedAt:    j.ClosedAt,
	}
	if j.SalaryMin != nil || j.SalaryMax != nil {
		r.Salary = &salaryResponse{Min: j.SalaryMin, Max: j.SalaryMax, Currency: j.SalaryCurrency}
	}
	if maxDesc > 0 {
		r.Description = truncate(r.Description, maxDesc)
	}
	return r
}

// truncate cuts s to at most n characters (runes, not bytes, so Arabic or
// accented text is never cut in the middle of a character).
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
