// Package ats scrapes company job boards hosted on applicant tracking
// systems (ATS) like Greenhouse and Workable.
//
// Companies post openings in their ATS, which powers their careers page
// through a public JSON feed. We read that feed: it's the original source
// the big aggregators copy from, it's meant to be read by programs, and it
// lists every open job at the company. That last part matters: a job that
// disappears from the feed has closed.
package ats

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"job-scraper-go/internal/scraper"
)

// Board is one company's job board, as listed in companies.yaml.
type Board struct {
	Company string `yaml:"company"` // display name, e.g. "Careem"
	ATS     string `yaml:"ats"`     // which system hosts the board
	Slug    string `yaml:"slug"`    // the company's account name in that system
}

// New returns the scraper for a board's ATS.
func New(b Board, client *scraper.Client) (scraper.Scraper, error) {
	base := base{board: b, client: client}
	switch b.ATS {
	case "greenhouse":
		base.baseURL = "https://boards-api.greenhouse.io"
		return &greenhouse{base}, nil
	case "ashby":
		base.baseURL = "https://api.ashbyhq.com"
		return &ashby{base}, nil
	case "workable":
		base.baseURL = "https://apply.workable.com"
		return &workable{base}, nil
	case "smartrecruiters":
		base.baseURL = "https://api.smartrecruiters.com"
		return &smartRecruiters{base}, nil
	case "recruitee":
		base.baseURL = "https://" + b.Slug + ".recruitee.com"
		return &recruitee{base}, nil
	}
	return nil, fmt.Errorf("company %q: unknown ats %q", b.Company, b.ATS)
}

// base holds what every ATS scraper needs. Each scraper type embeds it, so
// they all get Name() and externalID() without repeating them.
type base struct {
	board   Board
	client  *scraper.Client
	baseURL string // overridden in tests to point at a local server
}

func (b base) Name() string { return b.board.ATS + "/" + b.board.Slug }

// externalID prefixes the ATS's job ID with the company slug. IDs are only
// guaranteed unique within one company's board, not across the whole ATS.
func (b base) externalID(id string) string { return b.board.Slug + ":" + id }

// Slugs end up in URLs (and, for Recruitee, in the hostname), so only allow
// characters that are safe there.
var validSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// LoadBoards reads and validates the companies file.
func LoadBoards(path string) ([]Board, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var file struct {
		Companies []Board `yaml:"companies"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	seen := make(map[string]bool)
	for i, b := range file.Companies {
		if b.Company == "" || b.ATS == "" || !validSlug.MatchString(b.Slug) {
			return nil, fmt.Errorf("%s: entry %d needs company, ats and a lowercase slug", path, i+1)
		}
		key := b.ATS + "/" + b.Slug
		if seen[key] {
			return nil, fmt.Errorf("%s: %s is listed twice", path, key)
		}
		seen[key] = true
	}
	return file.Companies, nil
}

// Helpers shared by the ATS scrapers.

func isRemote(location string) bool {
	return strings.Contains(strings.ToLower(location), "remote")
}

// parseTime returns the first value that parses with layout, or nil. ATS
// feeds often have several date fields, some of them empty.
func parseTime(layout string, values ...string) *time.Time {
	for _, v := range values {
		if t, err := time.Parse(layout, strings.TrimSpace(v)); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

// joinNonEmpty joins the non-blank parts: ("Dubai", "", "UAE") -> "Dubai, UAE".
func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}
