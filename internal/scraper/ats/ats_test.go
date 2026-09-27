package ats

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/scraper"
)

// The fixtures below are trimmed copies of real responses from each ATS,
// keeping the quirks the scrapers have to handle.

const greenhouseFixture = `{"jobs": [{
	"id": 4969403101,
	"title": "Senior Accountant ",
	"absolute_url": "https://job-boards.eu.greenhouse.io/acme/jobs/4969403101",
	"content": "&lt;h3&gt;About Us&lt;/h3&gt;&lt;p&gt;We build &amp;amp; ship.&lt;/p&gt;",
	"first_published": "2026-09-09T04:48:17-04:00",
	"updated_at": "2026-09-17T14:37:50-04:00",
	"location": {"name": "Dubai, United Arab Emirates"}
}]}`

const ashbyFixture = `{"jobs": [
	{
		"id": "b52d240f", "title": "Backend Engineer", "location": "Cairo Office",
		"isListed": true, "isRemote": true, "workplaceType": "Hybrid",
		"publishedAt": "2026-08-12T05:44:50.125+00:00",
		"jobUrl": "https://jobs.ashbyhq.com/acme/b52d240f",
		"descriptionPlain": "Build   payments\n\ninfrastructure.",
		"address": {"postalAddress": {"addressCountry": "Egypt"}}
	},
	{"id": "hidden", "title": "Unlisted", "isListed": false}
]}`

const workableFixture = `{"name": "Acme", "jobs": [{
	"title": "Anti Fraud Officer", "shortcode": "8B8A457674",
	"url": "https://apply.workable.com/j/8B8A457674",
	"published_on": "2026-09-21", "city": "Riyadh", "country": "Saudi Arabia",
	"telecommuting": false, "experience": "Entry level",
	"description": "<p>Fight fraud.</p>",
	"locations": [{"countryCode": "SA"}]
}]}`

const srListFixture = `{"totalFound": 1, "content": [{
	"id": "743999663339278", "name": "Stylist",
	"releasedDate": "2017-12-05T10:14:13.000Z",
	"company": {"identifier": "Acme"},
	"location": {"country": "ae", "remote": false, "fullLocation": "Dubai, , United Arab Emirates"},
	"experienceLevel": {"id": "mid_senior_level"}
}]}`

const srDetailFixture = `{
	"postingUrl": "https://jobs.smartrecruiters.com/Acme/743999663339278-stylist",
	"jobAd": {"sections": {
		"companyDescription": {"text": "<p>Boilerplate about Acme.</p>"},
		"jobDescription": {"text": "<p>Style outfits.</p>"},
		"qualifications": {"text": "<ul><li>Taste</li></ul>"}
	}}
}`

const recruiteeFixture = `{"offers": [{
	"id": 2736919, "title": "Strategic Account Manager",
	"careers_url": "https://jobs.acme.com/o/strategic-account-manager",
	"location": "Riyadh, Riyadh Province, Saudi Arabia", "country_code": "SA",
	"remote": false, "description": "<p>Grow accounts.</p>", "requirements": "<p>Sales.</p>",
	"published_at": "2026-09-10 07:25:25 UTC", "experience_code": "experienced",
	"salary": {"min": "240000", "max": 300000, "period": "year", "currency": "sar"}
}]}`

// fakeATS serves every fixture at the path the real ATS uses.
func fakeATS(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	serve := func(path, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
	}
	serve("GET /v1/boards/acme/jobs", greenhouseFixture)
	serve("GET /posting-api/job-board/acme", ashbyFixture)
	serve("GET /api/v1/widget/accounts/acme", workableFixture)
	serve("GET /v1/companies/acme/postings", srListFixture)
	serve("GET /v1/companies/acme/postings/743999663339278", srDetailFixture)
	serve("GET /api/offers/", recruiteeFixture)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func scrapeOne(t *testing.T, ats string) job.Job {
	t.Helper()
	srv := fakeATS(t)

	s, err := New(Board{Company: "Acme", ATS: ats, Slug: "acme"}, scraper.NewClient(scraper.ClientOptions{
		RequestsPerSecond: 1000, Timeout: 5 * time.Second,
	}))
	if err != nil {
		t.Fatal(err)
	}
	// Point the scraper at the fake server. The test is in the same package,
	// so it can reach the unexported field; production code can't.
	switch s := s.(type) {
	case *greenhouse:
		s.baseURL = srv.URL
	case *ashby:
		s.baseURL = srv.URL
	case *workable:
		s.baseURL = srv.URL
	case *smartRecruiters:
		s.baseURL = srv.URL
	case *recruitee:
		s.baseURL = srv.URL
	}

	jobs, err := s.Scrape(context.Background())
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Company != "Acme" {
		t.Errorf("Company = %q, want the name from companies.yaml", jobs[0].Company)
	}
	return jobs[0]
}

func TestGreenhouse(t *testing.T) {
	j := scrapeOne(t, "greenhouse")
	if j.ExternalID != "acme:4969403101" || j.Title != "Senior Accountant" {
		t.Errorf("id=%q title=%q", j.ExternalID, j.Title)
	}
	if j.Description != "About Us We build & ship." {
		t.Errorf("Description = %q (double-escaped HTML not handled?)", j.Description)
	}
	if j.PostedAt == nil || j.PostedAt.Format(time.RFC3339) != "2026-09-09T08:48:17Z" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
}

func TestAshby(t *testing.T) {
	j := scrapeOne(t, "ashby") // the unlisted job must be skipped
	if j.Country != "Egypt" || j.Remote {
		t.Errorf("country=%q remote=%v; want Egypt from address, hybrid is not remote", j.Country, j.Remote)
	}
	if j.Description != "Build payments infrastructure." {
		t.Errorf("Description = %q", j.Description)
	}
}

func TestWorkable(t *testing.T) {
	j := scrapeOne(t, "workable")
	if j.Country != "SA" || j.Location != "Riyadh, Saudi Arabia" || j.Seniority != job.SeniorityJunior {
		t.Errorf("country=%q location=%q seniority=%q", j.Country, j.Location, j.Seniority)
	}
}

func TestSmartRecruiters(t *testing.T) {
	j := scrapeOne(t, "smartrecruiters")
	if j.URL != "https://jobs.smartrecruiters.com/Acme/743999663339278-stylist" {
		t.Errorf("URL = %q, want the one from the detail request", j.URL)
	}
	if j.Description != "Style outfits. Taste" {
		t.Errorf("Description = %q, want sections without company boilerplate", j.Description)
	}
	if j.Location != "Dubai, United Arab Emirates" {
		t.Errorf("Location = %q", j.Location)
	}
}

func TestRecruitee(t *testing.T) {
	j := scrapeOne(t, "recruitee")
	if j.SalaryMin == nil || *j.SalaryMin != 240000 || j.SalaryMax == nil || *j.SalaryMax != 300000 {
		t.Errorf("salary = %v-%v, want 240000-300000 (string and number forms)", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "SAR" {
		t.Errorf("SalaryCurrency = %q", j.SalaryCurrency)
	}
}

func TestLoadBoards(t *testing.T) {
	write := func(content string) string {
		path := filepath.Join(t.TempDir(), "companies.yaml")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	boards, err := LoadBoards(write(`
companies:
  - {company: Careem, ats: greenhouse, slug: careem}
  - {company: Salla, ats: workable, slug: salla}
`))
	if err != nil || len(boards) != 2 || boards[1].Company != "Salla" {
		t.Fatalf("boards=%+v err=%v", boards, err)
	}

	bad := map[string]string{
		"duplicate":   "companies:\n  - {company: A, ats: greenhouse, slug: a}\n  - {company: B, ats: greenhouse, slug: a}",
		"unsafe slug": "companies:\n  - {company: A, ats: recruitee, slug: 'evil.com/x'}",
		"missing ats": "companies:\n  - {company: A, slug: a}",
	}
	for name, content := range bad {
		if _, err := LoadBoards(write(content)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
