// Package scraper defines what a job source looks like and runs many of them
// concurrently. Each job board lives in its own subpackage and implements
// the Scraper interface.
package scraper

import (
	"context"
	"html"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"

	"job-scraper-go/internal/job"
)

// Scraper is the one interface every source implements. It is intentionally
// small: the runner only needs a name for logging and a way to fetch jobs.
type Scraper interface {
	// Name identifies the feed, e.g. "remoteok" or "greenhouse/careem".
	Name() string
	Scrape(ctx context.Context) ([]job.Job, error)
}

// UserAgent identifies us honestly to the sites we fetch from.
const UserAgent = "job-scraper-go/0.1 (learning project)"

// CleanText trims whitespace and repairs broken encoding in a short field
// like a title or company name. That includes HTML entities leaking into
// plain-text fields: Lucidya's feed has a title with "&amp" in it.
func CleanText(s string) string {
	return FixMojibake(strings.TrimSpace(html.UnescapeString(s)))
}

// FixMojibake repairs text that was UTF-8 but got decoded as Latin-1 somewhere
// upstream, turning "á" into "Ã¡". Remote OK's API does this to some titles.
//
// The repair maps each character back to one byte and checks whether those
// bytes form valid UTF-8. Real Latin-1 text like "café" produces invalid
// UTF-8 when treated that way, so it is left alone.
func FixMojibake(s string) string {
	b := make([]byte, 0, len(s))
	multibyte := false
	for _, r := range s {
		if r > 0xFF {
			return s // has characters Latin-1 can't hold, so it isn't this bug
		}
		if r >= 0x80 {
			multibyte = true
		}
		b = append(b, byte(r))
	}
	if !multibyte || !utf8.Valid(b) {
		return s
	}
	return string(b)
}

// HTMLToText strips tags from an HTML fragment and collapses whitespace.
func HTMLToText(fragment string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fragment))
	if err != nil {
		return fragment
	}
	// Put a space between block elements so "<p>a</p><p>b</p>" becomes
	// "a b" rather than "ab".
	doc.Find("p, li, br, div, h1, h2, h3, h4").Each(func(_ int, s *goquery.Selection) {
		s.AppendHtml(" ")
	})
	return strings.Join(strings.Fields(doc.Text()), " ")
}
