package scraper

import (
	"html"
	"strings"
	"time"
	"unicode/utf8"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Helpers that clean raw fields from a source before they become a job.Job.

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
	doc, err := nethtml.Parse(strings.NewReader(fragment))
	if err != nil {
		return fragment
	}
	var b strings.Builder
	writeText(&b, doc)
	return strings.Join(strings.Fields(b.String()), " ")
}

// blockElements end with a space, so "<p>a</p><p>b</p>" becomes "a b"
// rather than "ab". Inline elements don't: "Sen<b>ior</b>" stays one word.
var blockElements = map[atom.Atom]bool{
	atom.P: true, atom.Li: true, atom.Br: true, atom.Div: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true,
}

// writeText writes the text inside n, in document order.
func writeText(b *strings.Builder, n *nethtml.Node) {
	if n.Type == nethtml.TextNode {
		b.WriteString(n.Data)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeText(b, c)
	}
	if n.Type == nethtml.ElementNode && blockElements[n.DataAtom] {
		b.WriteByte(' ')
	}
}

// ParseTime returns the first value that parses with layout, in UTC, or nil.
// Feeds often have several date fields, some of them empty.
func ParseTime(layout string, values ...string) *time.Time {
	for _, v := range values {
		if t, err := time.Parse(layout, strings.TrimSpace(v)); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

// JoinNonEmpty joins the non-blank parts: ("Dubai", "", "UAE") -> "Dubai, UAE".
func JoinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}
