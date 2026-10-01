package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"job-scraper-go/internal/job"
)

var ErrNotFound = errors.New("not found")

// JobRecord is a stored job: the scraped fields plus what the database adds.
type JobRecord struct {
	ID int64
	job.Job
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	ClosedAt    *time.Time
}

const (
	SortRelevance = "relevance"
	SortNewest    = "newest"
)

// SearchParams are already validated by the caller. Empty fields mean "don't
// filter on this".
type SearchParams struct {
	Query         string    // full-text search over title, company, description
	Seniority     []string  // any of these levels
	Countries     []string  // any of these ISO codes
	Skills        []string  // all of these skills
	Location      string    // substring of the location text, e.g. "Riyadh"
	Company       string    // substring of the company name
	Remote        *bool     // nil = either
	Tech          *bool     // nil = either; see job.IsTech
	PostedSince   time.Time // zero = any age
	IncludeClosed bool
	Sort          string
	Limit         int
	Offset        int
	// DescriptionLen loads at most this many characters of each job's
	// description, 0 for all of it. A results list only shows the start,
	// and descriptions run to many kilobytes: cutting them in the database
	// keeps them from being read and sent over only to be thrown away.
	DescriptionLen int
}

type SearchResult struct {
	Total int // matches across all pages
	Jobs  []JobRecord
}

var jobColumns = jobColumnsWith("description")

// jobColumnsWith lists the columns scanJob reads, with description being
// the expression that loads the description.
func jobColumnsWith(description string) string {
	return `
	id, source, external_id, board, title, company, location, country, remote,
	seniority, url, ` + description + `, tags, skills, tech, salary_min, salary_max, salary_currency,
	posted_at, first_seen_at, last_seen_at, closed_at`
}

func scanJob(row pgx.Row) (JobRecord, error) {
	var r JobRecord
	var seniority string
	err := row.Scan(
		&r.ID, &r.Source, &r.ExternalID, &r.Board, &r.Title, &r.Company, &r.Location, &r.Country, &r.Remote,
		&seniority, &r.URL, &r.Description, &r.Tags, &r.Skills, &r.Tech, &r.SalaryMin, &r.SalaryMax, &r.SalaryCurrency,
		&r.PostedAt, &r.FirstSeenAt, &r.LastSeenAt, &r.ClosedAt,
	)
	r.Seniority = job.Seniority(seniority)
	return r, err
}

// filter is a WHERE clause and the query parameters it refers to.
type filter struct {
	where string
	args  []any
	rank  string // relevance expression when there's a text query, else ""
}

// arg stores a value as a query parameter and returns its placeholder ($1,
// $2, ...). User input only ever reaches SQL this way, never by pasting it
// into the query string, which is what rules out SQL injection.
func (f *filter) arg(v any) string {
	f.args = append(f.args, v)
	return fmt.Sprintf("$%d", len(f.args))
}

// buildFilter turns whichever search params are set into a WHERE clause.
// Search and stats share it, so the stats always describe exactly the jobs
// a search with the same params would return.
func buildFilter(p SearchParams) *filter {
	f := &filter{}
	var conds []string

	if p.Query != "" {
		// websearch_to_tsquery understands what people type into search
		// boxes: `backend engineer`, `"data analyst"`, `python -django`.
		tsq := "websearch_to_tsquery('english', " + f.arg(p.Query) + ")"
		conds = append(conds, "search @@ "+tsq)
		f.rank = "ts_rank(search, " + tsq + ")"
	}
	if !p.IncludeClosed {
		conds = append(conds, "closed_at IS NULL")
	}
	if len(p.Seniority) > 0 {
		conds = append(conds, "seniority = ANY("+f.arg(p.Seniority)+")")
	}
	if len(p.Countries) > 0 {
		conds = append(conds, "country = ANY("+f.arg(p.Countries)+")")
	}
	if len(p.Skills) > 0 {
		// @> is "contains", which the GIN index on skills answers directly.
		conds = append(conds, "skills @> "+f.arg(p.Skills))
	}
	if p.Location != "" {
		conds = append(conds, "location ILIKE "+f.arg("%"+escapeLike(p.Location)+"%"))
	}
	if p.Company != "" {
		conds = append(conds, "company ILIKE "+f.arg("%"+escapeLike(p.Company)+"%"))
	}
	if p.Remote != nil {
		conds = append(conds, "remote = "+f.arg(*p.Remote))
	}
	if p.Tech != nil {
		conds = append(conds, "tech = "+f.arg(*p.Tech))
	}
	if !p.PostedSince.IsZero() {
		conds = append(conds, postedSinceCond+" >= "+f.arg(p.PostedSince))
	}

	if len(conds) > 0 {
		f.where = "WHERE " + strings.Join(conds, " AND ")
	}
	return f
}

// SearchJobs returns one page of the jobs matching p, plus the total count.
func (s *Store) SearchJobs(ctx context.Context, p SearchParams) (SearchResult, error) {
	f := buildFilter(p)

	// The count and the page go to the database as one batch, so a search
	// costs a single round trip. The count takes only the filter's
	// parameters, so it gets a copy made before the page adds its own.
	batch := &pgx.Batch{}
	batch.Queue("SELECT count(*) FROM jobs "+f.where, slices.Clone(f.args)...)

	// id is the final tiebreaker so the order is fully deterministic. Without
	// it, rows with equal dates can swap between requests, and paging would
	// show some jobs twice and skip others.
	order := "posted_at DESC NULLS LAST, id DESC"
	if p.Sort == SortRelevance && f.rank != "" {
		order = f.rank + " DESC, " + order
	}

	columns := jobColumns
	if p.DescriptionLen > 0 {
		columns = jobColumnsWith("left(description, " + f.arg(p.DescriptionLen) + ")")
	}
	batch.Queue(fmt.Sprintf("SELECT %s FROM jobs %s ORDER BY %s LIMIT %s OFFSET %s",
		columns, f.where, order, f.arg(p.Limit), f.arg(p.Offset)), f.args...)

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	var res SearchResult
	if err := br.QueryRow().Scan(&res.Total); err != nil {
		return res, fmt.Errorf("count jobs: %w", err)
	}
	rows, err := br.Query()
	if err != nil {
		return res, fmt.Errorf("search jobs: %w", err)
	}
	res.Jobs, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobRecord, error) {
		return scanJob(row)
	})
	if err != nil {
		return res, fmt.Errorf("scan jobs: %w", err)
	}
	return res, br.Close()
}

type CountryCount struct {
	Code  string
	Count int
}

// postedSinceCond is a job's age for filtering: its post date, or when we
// first saw it for sources that don't give one.
const postedSinceCond = "COALESCE(posted_at, first_seen_at)"

// CountryCounts returns how many open jobs each country has, most first,
// counting only jobs posted since the given time, and only tech jobs when
// techOnly is set. The search page uses it for its country dropdown, so the
// counts match what a search would return.
func (s *Store) CountryCounts(ctx context.Context, postedSince time.Time, techOnly bool) ([]CountryCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT country, count(*) FROM jobs
		WHERE closed_at IS NULL AND country <> '' AND `+postedSinceCond+` >= $1
		  AND (tech OR NOT $2)
		GROUP BY country
		ORDER BY count(*) DESC, country`, postedSince, techOnly)
	if err != nil {
		return nil, fmt.Errorf("count countries: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[CountryCount])
}

func (s *Store) GetJob(ctx context.Context, id int64) (JobRecord, error) {
	r, err := scanJob(s.pool.QueryRow(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// escapeLike stops user input from acting as LIKE wildcards: searching for
// "100%" should match the literal text, not "100 followed by anything".
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
