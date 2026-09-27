package api

import (
	"math"
	"time"

	"job-scraper-go/internal/job"
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
	Tech        bool            `json:"tech"`
	Level       string          `json:"level"`
	URL         string          `json:"url"`
	Source      string          `json:"source"`
	Salary      *salaryResponse `json:"salary,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Skills      []string        `json:"skills,omitempty"`
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
		Tech:        j.Tech,
		Level:       string(j.Seniority),
		URL:         j.URL,
		Source:      j.Source,
		Tags:        j.Tags,
		Skills:      j.Skills,
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

type statsResponse struct {
	Total        int             `json:"total"`
	Levels       []levelCount    `json:"levels"`
	TopCompanies []companyCount  `json:"top_companies"`
	TopSkills    []skillResponse `json:"top_skills"`
}

type levelCount struct {
	Level string `json:"level"`
	Count int    `json:"count"`
}

type companyCount struct {
	Company string `json:"company"`
	Count   int    `json:"count"`
}

type skillResponse struct {
	Skill    string `json:"skill"`
	Category string `json:"category"`
	Count    int    `json:"count"`
	// Share is the fraction of matching jobs that mention the skill, which
	// reads better than a raw count: "40% of these jobs ask for SQL".
	Share float64 `json:"share"`
}

func toStatsResponse(st store.Stats) statsResponse {
	r := statsResponse{
		Total:        st.Total,
		Levels:       make([]levelCount, 0, len(job.Seniorities)),
		TopCompanies: make([]companyCount, 0, len(st.Companies)),
		TopSkills:    make([]skillResponse, 0, len(st.Skills)),
	}

	// Every level, in order from intern to executive, including the ones
	// with no jobs. A client drawing a chart gets the same axis every time.
	counts := make(map[string]int, len(st.Levels))
	for _, l := range st.Levels {
		counts[l.Name] = l.Count
	}
	for _, level := range job.Seniorities {
		r.Levels = append(r.Levels, levelCount{string(level), counts[string(level)]})
	}

	for _, c := range st.Companies {
		r.TopCompanies = append(r.TopCompanies, companyCount{c.Name, c.Count})
	}
	for _, s := range st.Skills {
		skill, _ := job.LookupSkill(s.Name)
		r.TopSkills = append(r.TopSkills, skillResponse{
			Skill:    s.Name,
			Category: skill.Category,
			Count:    s.Count,
			Share:    math.Round(float64(s.Count)/float64(max(st.Total, 1))*1000) / 1000,
		})
	}
	return r
}
