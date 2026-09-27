// Package job defines the unified job posting schema that every source is
// normalized into. Scrapers produce Jobs; the store persists them; the API
// serves them. Nothing in here knows about HTTP, HTML, or SQL.
package job

import (
	"strings"
	"time"
)

type Job struct {
	// Source + ExternalID together identify a posting. ExternalID is whatever
	// ID the job board uses, so re-scraping the same posting updates the
	// existing row instead of creating a duplicate.
	Source     string
	ExternalID string

	// Board is the feed the job came from, e.g. "greenhouse/careem". The
	// scraper command sets it; the store uses it to close jobs that vanish.
	Board string

	Title       string
	Company     string // "" when the employer is hidden
	Location    string // free text as the source wrote it: "Riyadh, Saudi Arabia"
	Country     string // ISO 3166-1 alpha-2 code like "AE"; "" when unknown
	Remote      bool
	Seniority   Seniority
	URL         string
	Description string // plain text, HTML stripped
	Tags        []string
	Skills      []string // detected from the title and description, see skills.go

	// Salary fields are pointers because "unknown" is different from zero.
	// Most postings don't publish a salary, and we must not average in 0s.
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string

	PostedAt *time.Time
}

// Normalize fills in the fields every source needs but not every source
// provides, using the same rules for all of them. Scrapers set whatever
// structured data their source gives them; this fills the gaps.
func (j *Job) Normalize() {
	// An explicit word in the title ("Senior", "Intern") beats the source's
	// own level field, which companies often leave on a default value.
	if s := SeniorityFromTitle(j.Title); s != "" {
		j.Seniority = s
	} else if j.Seniority == "" {
		j.Seniority = SeniorityMid
	}

	// Scrapers may put a country name or code here from a structured field.
	// Turn it into a code, or fall back to reading the location text.
	j.Country = CountryCode(j.Country)
	if j.Country == "" {
		j.Country = CountryFromText(j.Location)
	}

	// Hidden employers show up under a placeholder name, and 361 unrelated
	// Workable jobs in Egypt were all "Company". Stored as-is, the placeholder
	// would top every "who's hiring" chart, so it becomes "" (unknown).
	if placeholderCompanies[strings.ToLower(strings.TrimSpace(j.Company))] {
		j.Company = ""
	}

	j.Skills = SkillsFromText(j.Title + "\n" + j.Description)
}

var placeholderCompanies = map[string]bool{
	"company": true, "confidential": true, "confidential company": true, "undisclosed": true,
}
