package main

import (
	"maps"
	"testing"
)

func TestParseLine(t *testing.T) {
	got := parseLine(`time=2026-10-05T16:09:26.102Z level=INFO msg="job click" id=3 title="Senior \"Go\" Engineer" company=""`)
	want := map[string]string{
		"time": "2026-10-05T16:09:26.102Z", "level": "INFO", "msg": "job click",
		"id": "3", "title": `Senior "Go" Engineer`, "company": "",
	}
	if !maps.Equal(got, want) {
		t.Errorf("parseLine = %q, want %q", got, want)
	}

	// Lines that aren't key=value, like a panic trace, give what they can.
	if got := parseLine("goroutine 1 [running]:"); len(got) != 0 {
		t.Errorf("parseLine(non-log line) = %q, want nothing", got)
	}
}

// A day of logs as the API writes them: two visitors, plus noise that must
// not count.
var sampleLog = []string{
	`time=2026-10-05T09:00:00Z level=INFO msg=listening url=http://localhost:8080`,
	`time=2026-10-05T09:00:01Z level=INFO msg=request method=GET path=/healthz query="" status=200 duration=1ms ip=172.18.0.1`,
	`time=2026-10-05T10:00:00Z level=INFO msg=request method=GET path=/ query="" status=200 duration=1ms ip=10.0.0.1`,
	`time=2026-10-05T10:00:00Z level=INFO msg=request method=GET path=/static/app.js query="" status=200 duration=1ms ip=10.0.0.1`,
	`time=2026-10-05T10:00:01Z level=INFO msg=request method=GET path=/jobs query="limit=20" status=200 duration=9ms ip=10.0.0.1`,
	`time=2026-10-05T10:01:00Z level=INFO msg=request method=GET path=/jobs query="q=Backend++Engineer&country=AE%2CSA&tech=true&limit=20" status=200 duration=9ms ip=10.0.0.1`,
	`time=2026-10-05T10:01:30Z level=INFO msg=request method=GET path=/jobs query="q=backend+engineer&country=AE%2CSA&tech=true&limit=20&page=2" status=200 duration=9ms ip=10.0.0.1`,
	`time=2026-10-05T10:02:00Z level=INFO msg="job click" id=42 title="Backend Engineer" company=Careem`,
	`time=2026-10-05T10:02:00Z level=INFO msg=request method=POST path=/jobs/42/click query="" status=204 duration=2ms ip=10.0.0.1`,
	`time=2026-10-05T11:00:00Z level=INFO msg=request method=GET path=/ query="" status=200 duration=1ms ip=10.0.0.2`,
	`time=2026-10-05T11:00:05Z level=INFO msg=request method=GET path=/jobs query="q=backend+engineer&remote=true&limit=20" status=200 duration=9ms ip=10.0.0.2`,
	`time=2026-10-05T11:00:09Z level=INFO msg=request method=POST path=/jobs/42/click query="" status=204 duration=2ms ip=10.0.0.2`,
}

func TestSummarize(t *testing.T) {
	s := summarize(sampleLog, nil)

	if len(s.visitors) != 2 {
		t.Fatalf("visitors = %d, want 2 (the health check's IP doesn't count)", len(s.visitors))
	}
	if got, want := s.visitors["10.0.0.1"].counts, (counts{pages: 1, searches: 1, clicks: 1}); got != want {
		t.Errorf("10.0.0.1 = %+v, want %+v (page 2 is the same search)", got, want)
	}
	if got := s.searches["backend engineer"]; got != 2 {
		t.Errorf(`searches["backend engineer"] = %d, want 2 (case and spaces ignored)`, got)
	}
	if got := s.clicks["Backend Engineer, at Careem"]; got != 2 {
		t.Errorf("clicks = %v, want Backend Engineer, at Careem: 2", s.clicks)
	}
	wantFilters := map[string]int{"country AE": 1, "country SA": 1, "remote only": 1, "tech jobs only": 1}
	if !maps.Equal(s.filters, wantFilters) {
		t.Errorf("filters = %v, want %v", s.filters, wantFilters)
	}
	if len(s.days) != 1 {
		t.Errorf("days = %d, want 1", len(s.days))
	}
}

func TestSummarizeIgnore(t *testing.T) {
	s := summarize(sampleLog, map[string]bool{"10.0.0.1": true})
	if len(s.visitors) != 1 || s.visitors["10.0.0.2"] == nil {
		t.Errorf("visitors = %v, want only 10.0.0.2", s.visitors)
	}
	if got := s.searches["backend engineer"]; got != 1 {
		t.Errorf("searches = %v, want only 10.0.0.2's", s.searches)
	}
}
