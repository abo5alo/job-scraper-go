package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"job-scraper-go/internal/job"
	"job-scraper-go/internal/store"
)

// fakeStore records what the handler asked for and returns canned data.
type fakeStore struct {
	gotParams store.SearchParams
	result    store.SearchResult
}

func (f *fakeStore) SearchJobs(_ context.Context, p store.SearchParams) (store.SearchResult, error) {
	f.gotParams = p
	return f.result, nil
}

func (f *fakeStore) GetJob(_ context.Context, id int64) (store.JobRecord, error) {
	for _, j := range f.result.Jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return store.JobRecord{}, store.ErrNotFound
}

func (f *fakeStore) CountryCounts(context.Context, time.Time) ([]store.CountryCount, error) {
	return []store.CountryCount{{Code: "SA", Count: 76}}, nil
}

func (f *fakeStore) Stats(_ context.Context, p store.SearchParams, _ int) (store.Stats, error) {
	f.gotParams = p
	return store.Stats{
		Total:     10,
		Levels:    []store.NameCount{{Name: "senior", Count: 4}, {Name: "mid", Count: 6}},
		Companies: []store.NameCount{{Name: "Careem", Count: 3}},
		Skills:    []store.NameCount{{Name: "SQL", Count: 4}, {Name: "Arabic", Count: 1}},
	}, nil
}

func (f *fakeStore) Ping(context.Context) error { return nil }

func TestSearchPage(t *testing.T) {
	h := newTestHandler(&fakeStore{})

	rec := get(t, h, "/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<form id="search"`) {
		t.Fatalf("GET /: %d, body starts %.80q", rec.Code, rec.Body)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
	if rec := get(t, h, "/static/app.js"); rec.Code != http.StatusOK {
		t.Errorf("GET /static/app.js: %d", rec.Code)
	}
	if rec := get(t, h, "/countries"); !strings.Contains(rec.Body.String(), `[{"code":"SA","count":76}]`) {
		t.Errorf("GET /countries: %s", rec.Body)
	}
}

func newTestHandler(st JobStore) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(st, log, Options{RequestsPerSecond: 1000, Burst: 1000})
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSearchJobs(t *testing.T) {
	st := &fakeStore{result: store.SearchResult{Total: 1, Jobs: []store.JobRecord{{
		ID: 7,
		Job: job.Job{
			Title: "Senior Backend Engineer", Company: "Careem", Country: "AE",
			Seniority: job.SenioritySenior, URL: "https://example.com/7", Source: "greenhouse",
			Description: strings.Repeat("x", 500),
		},
		FirstSeenAt: time.Now(),
	}}}}

	rec := get(t, newTestHandler(st), "/jobs?q=backend&level=senior&country=uae,Saudi%20Arabia&remote=false&page=2&limit=10")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	// The handler should turn the query string into validated params.
	p := st.gotParams
	if p.Query != "backend" || p.Sort != store.SortRelevance || p.Limit != 10 || p.Offset != 10 {
		t.Errorf("params = %+v", p)
	}
	if !slices.Equal(p.Countries, []string{"AE", "SA"}) || !slices.Equal(p.Seniority, []string{"senior"}) {
		t.Errorf("countries=%v seniority=%v", p.Countries, p.Seniority)
	}
	if p.Remote == nil || *p.Remote {
		t.Errorf("remote = %v, want false", p.Remote)
	}
	if age := time.Since(p.PostedSince); age < maxJobAge-time.Minute || age > maxJobAge+time.Minute {
		t.Errorf("PostedSince is %v ago, want about %v", age, maxJobAge)
	}

	var body searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || body.Page != 2 || len(body.Jobs) != 1 {
		t.Fatalf("body = %+v", body)
	}
	if got := body.Jobs[0]; got.Level != "senior" || got.URL != "https://example.com/7" {
		t.Errorf("job = %+v", got)
	}
	if n := len([]rune(body.Jobs[0].Description)); n != listDescriptionLen+1 { // +1 for "…"
		t.Errorf("description length = %d, want truncated to %d", n, listDescriptionLen)
	}
}

func TestSearchJobsEmptyIsArray(t *testing.T) {
	rec := get(t, newTestHandler(&fakeStore{}), "/jobs")
	if !strings.Contains(rec.Body.String(), `"jobs":[]`) {
		t.Errorf("body = %s; want an empty array, not null", rec.Body)
	}
}

func TestSearchJobsValidation(t *testing.T) {
	h := newTestHandler(&fakeStore{})
	bad := []string{
		"level=wizard",
		"country=Atlantis",
		"remote=maybe",
		"page=0",
		"limit=1000",
		"sort=salary",
		"skill=cobol",
		"q=" + strings.Repeat("a", maxQueryLen+1),
	}
	for _, q := range bad {
		rec := get(t, h, "/jobs?"+q)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("?%s: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestStats(t *testing.T) {
	st := &fakeStore{}
	rec := get(t, newTestHandler(st), "/stats?country=AE&skill=sql,python")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	// Same filters as /jobs, with skill names made canonical.
	p := st.gotParams
	if !slices.Equal(p.Countries, []string{"AE"}) || !slices.Equal(p.Skills, []string{"SQL", "Python"}) || p.PostedSince.IsZero() {
		t.Errorf("params = %+v", p)
	}

	var body statsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// All six levels in order, zeros included.
	if len(body.Levels) != 6 || body.Levels[0] != (levelCount{"intern", 0}) || body.Levels[3] != (levelCount{"senior", 4}) {
		t.Errorf("levels = %+v", body.Levels)
	}
	want := skillResponse{Skill: "SQL", Category: "programming language", Count: 4, Share: 0.4}
	if len(body.TopSkills) != 2 || body.TopSkills[0] != want {
		t.Errorf("top_skills = %+v, want first %+v", body.TopSkills, want)
	}
	if len(body.TopCompanies) != 1 || body.TopCompanies[0].Company != "Careem" {
		t.Errorf("top_companies = %+v", body.TopCompanies)
	}

	if rec := get(t, newTestHandler(st), "/stats?level=wizard"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad level: status = %d, want 400", rec.Code)
	}
}

func TestParseSearchParamsDefaults(t *testing.T) {
	p, page, err := parseSearchParams(url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if page != 1 || p.Limit != defaultLimit || p.Offset != 0 || p.Sort != store.SortNewest || p.IncludeClosed {
		t.Errorf("defaults = %+v page=%d", p, page)
	}
}

func TestGetJob(t *testing.T) {
	st := &fakeStore{result: store.SearchResult{Jobs: []store.JobRecord{{ID: 3, Job: job.Job{Title: "Accountant"}}}}}
	h := newTestHandler(st)

	if rec := get(t, h, "/jobs/3"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Accountant") {
		t.Errorf("GET /jobs/3: %d %s", rec.Code, rec.Body)
	}
	if rec := get(t, h, "/jobs/999"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /jobs/999: status = %d, want 404", rec.Code)
	}
	if rec := get(t, h, "/jobs/abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("GET /jobs/abc: status = %d, want 400", rec.Code)
	}
}

func TestRateLimitPerIP(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(&fakeStore{}, log, Options{RequestsPerSecond: 0.001, Burst: 2})

	request := func(ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// The burst of 2 is allowed, the third request is not.
	for i, want := range []int{200, 200, 429} {
		if got := request("10.0.0.1"); got != want {
			t.Errorf("request %d from 10.0.0.1: status = %d, want %d", i+1, got, want)
		}
	}
	// Another client has its own bucket and isn't affected.
	if got := request("10.0.0.2"); got != 200 {
		t.Errorf("request from 10.0.0.2: status = %d, want 200", got)
	}
}
