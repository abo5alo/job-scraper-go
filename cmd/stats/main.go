// Command stats reports who used the site, from the API's request log: the
// file the API writes when LOG_FILE is set (see docker-compose.tunnel.yml).
//
//	go run ./cmd/stats
//	go run ./cmd/stats -ignore 203.0.113.7   # leave out your own visits
//	go run ./cmd/stats -file path/to/api.log
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

func main() {
	file := flag.String("file", "logs/api.log", "the API's log file")
	ignore := flag.String("ignore", "", "comma-separated IPs to leave out, like your own")
	flag.Parse()

	lines, err := readLines(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ignored := map[string]bool{}
	for _, ip := range strings.Split(*ignore, ",") {
		if ip = strings.TrimSpace(ip); ip != "" {
			ignored[ip] = true
		}
	}

	fmt.Printf("Visitor stats from %s\n", *file)
	summarize(lines, ignored).print(os.Stdout)
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w\n\nThe API writes this file when it runs with LOG_FILE set, as it does with docker-compose.tunnel.yml", err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// counts are what's tallied per visitor and per day.
type counts struct {
	pages, searches, clicks int
}

type visitor struct {
	ip          string
	first, last time.Time
	counts
}

type day struct {
	date     string // "2006-01-02", so days sort by name
	visitors map[string]bool
	counts
}

type summary struct {
	first, last time.Time
	visitors    map[string]*visitor
	days        map[string]*day
	searches    map[string]int // search text, lowercased
	filters     map[string]int // e.g. "country AE", "remote only"
	clicks      map[string]int // "Title, at Company"
}

// clickPath matches the page's report of a click on job {id}.
var clickPath = regexp.MustCompile(`^/jobs/(\d+)/click$`)

// summarize tallies the request lines, leaving out health checks, static
// files and ignored IPs.
func summarize(lines []string, ignored map[string]bool) summary {
	s := summary{
		visitors: map[string]*visitor{},
		days:     map[string]*day{},
		searches: map[string]int{},
		filters:  map[string]int{},
		clicks:   map[string]int{},
	}

	// The API logs each click's job title on its own line, so collect the
	// titles first: a click's request line only has the job's id.
	titles := map[string]string{}
	for _, line := range lines {
		if f := parseLine(line); f["msg"] == "job click" {
			titles[f["id"]] = f["title"]
			if f["company"] != "" {
				titles[f["id"]] += ", at " + f["company"]
			}
		}
	}

	for _, line := range lines {
		f := parseLine(line)
		if f["msg"] != "request" {
			continue
		}
		path, ip := f["path"], f["ip"]
		if path == "/healthz" || strings.HasPrefix(path, "/static/") || ignored[ip] {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, f["time"])
		if err != nil {
			continue
		}
		t = t.Local()

		var c counts
		switch {
		case f["method"] == "GET" && path == "/" && f["status"] == "200":
			c.pages = 1
		case f["method"] == "GET" && path == "/jobs":
			c.searches = s.countSearch(f["query"])
		case f["method"] == "POST" && f["status"] == "204":
			if m := clickPath.FindStringSubmatch(path); m != nil {
				c.clicks = 1
				title := titles[m[1]]
				if title == "" {
					title = "job " + m[1]
				}
				s.clicks[title]++
			}
		}
		s.add(ip, t, c)
	}
	return s
}

// countSearch tallies the filters of one /jobs request, and its search text
// if it has any. It returns 1 when the request was a search. Requests for a
// later page of results are the same search, so they don't count again.
func (s *summary) countSearch(rawQuery string) int {
	q, err := url.ParseQuery(rawQuery)
	if err != nil || (q.Get("page") != "" && q.Get("page") != "1") {
		return 0
	}

	for _, name := range []string{"country", "level", "skill"} {
		for _, v := range strings.Split(q.Get(name), ",") {
			if v = strings.TrimSpace(v); v != "" {
				s.filters[name+" "+v]++
			}
		}
	}
	for _, name := range []string{"location", "company"} {
		if v := strings.TrimSpace(q.Get(name)); v != "" {
			s.filters[name+" "+strings.ToLower(v)]++
		}
	}
	if q.Get("remote") == "true" {
		s.filters["remote only"]++
	}
	if q.Get("tech") == "true" {
		s.filters["tech jobs only"]++
	}

	text := strings.Join(strings.Fields(strings.ToLower(q.Get("q"))), " ")
	if text == "" {
		return 0
	}
	s.searches[text]++
	return 1
}

func (s *summary) add(ip string, t time.Time, c counts) {
	if s.first.IsZero() || t.Before(s.first) {
		s.first = t
	}
	if t.After(s.last) {
		s.last = t
	}

	v := s.visitors[ip]
	if v == nil {
		v = &visitor{ip: ip, first: t}
		s.visitors[ip] = v
	}
	v.last = t
	v.counts = v.counts.plus(c)

	date := t.Format("2006-01-02")
	d := s.days[date]
	if d == nil {
		d = &day{date: date, visitors: map[string]bool{}}
		s.days[date] = d
	}
	d.visitors[ip] = true
	d.counts = d.counts.plus(c)
}

func (c counts) plus(o counts) counts {
	return counts{c.pages + o.pages, c.searches + o.searches, c.clicks + o.clicks}
}

func (s summary) print(w io.Writer) {
	if len(s.visitors) == 0 {
		fmt.Fprintln(w, "\nNo visits yet.")
		return
	}
	const stamp = "Jan 2 15:04"
	fmt.Fprintf(w, "%s to %s (this computer's time)\n", s.first.Format(stamp), s.last.Format(stamp))

	fmt.Fprintln(w, "\nBY DAY")
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "\tVisitors\tPage opens\tSearches\tJob clicks")
	var total counts
	for _, d := range sortedDays(s.days) {
		date, _ := time.Parse("2006-01-02", d.date)
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\n", date.Format("Mon Jan 2"), len(d.visitors), d.pages, d.searches, d.clicks)
		total = total.plus(d.counts)
	}
	fmt.Fprintf(tw, "Total\t%d\t%d\t%d\t%d\n", len(s.visitors), total.pages, total.searches, total.clicks)
	tw.Flush()

	fmt.Fprintln(w, "\nVISITORS (one IP can be one phone, or a whole household on the same Wi-Fi)")
	tw = tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "IP\tFirst seen\tLast seen\tPage opens\tSearches\tJob clicks")
	vs := make([]*visitor, 0, len(s.visitors))
	for _, v := range s.visitors {
		vs = append(vs, v)
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i].first.Before(vs[j].first) })
	for _, v := range vs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\n", v.ip, v.first.Format(stamp), v.last.Format(stamp), v.pages, v.searches, v.clicks)
	}
	tw.Flush()

	printTop(w, "TOP SEARCHES", s.searches)
	printTop(w, "FILTERS USED", s.filters)
	printTop(w, "MOST CLICKED JOBS", s.clicks)
}

func sortedDays(days map[string]*day) []*day {
	out := make([]*day, 0, len(days))
	for _, d := range days {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].date < out[j].date })
	return out
}

// printTop prints up to 15 entries, most frequent first.
func printTop(w io.Writer, title string, m map[string]int) {
	fmt.Fprintf(w, "\n%s\n", title)
	if len(m) == 0 {
		fmt.Fprintln(w, "  (none yet)")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys[:min(len(keys), 15)] {
		fmt.Fprintf(w, "  %4d  %s\n", m[k], k)
	}
}

// parseLine reads one line of slog's text format, key=value pairs where a
// value with spaces or special characters is a quoted Go string:
//
//	time=2026-10-05T16:09:26.102Z level=INFO msg=request path=/jobs query="q=go&limit=20"
//
// It returns the pairs it could read before anything malformed.
func parseLine(line string) map[string]string {
	fields := map[string]string{}
	for line = strings.TrimSpace(line); line != ""; line = strings.TrimLeft(line, " ") {
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			break
		}
		key, rest := line[:eq], line[eq+1:]

		var val string
		if strings.HasPrefix(rest, `"`) {
			end := closingQuote(rest)
			if end < 0 {
				break
			}
			v, err := strconv.Unquote(rest[:end+1])
			if err != nil {
				break
			}
			val, rest = v, rest[end+1:]
		} else {
			sp := strings.IndexByte(rest, ' ')
			if sp < 0 {
				sp = len(rest)
			}
			val, rest = rest[:sp], rest[sp:]
		}
		fields[key] = val
		line = rest
	}
	return fields
}

// closingQuote returns the index of the quote that ends the quoted string
// at the start of s, skipping escaped characters, or -1.
func closingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}
