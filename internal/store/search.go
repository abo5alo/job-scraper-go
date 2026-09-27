package store

import (
	"context"
	"errors"
	"fmt"
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
	Location      string    // substring of the location text, e.g. "Riyadh"
	Company       string    // substring of the company name
	Remote        *bool     // nil = either
	PostedSince   time.Time // zero = any age
	IncludeClosed bool
	Sort          string
	Limit         int
	Offset        int
}

type SearchResult struct {
	Total int // matches across all pages
	Jobs  []JobRecord
}

const jobColumns = `
	id, source, external_id, board, title, company, location, country, remote,
	seniority, url, description, tags, salary_min, salary_max, salary_currency,
	posted_at, first_seen_at, last_seen_at, closed_at`

func scanJob(row pgx.Row) (JobRecord, error) {
	var r JobRecord
	var seniority string
	err := row.Scan(
		&r.ID, &r.Source, &r.ExternalID, &r.Board, &r.Title, &r.Company, &r.Location, &r.Country, &r.Remote,
		&seniority, &r.URL, &r.Description, &r.Tags, &r.SalaryMin, &r.SalaryMax, &r.SalaryCurrency,
		&r.PostedAt, &r.FirstSeenAt, &r.LastSeenAt, &r.ClosedAt,
	)
	r.Seniority = job.Seniority(seniority)
	return r, err
}

// SearchJobs builds the WHERE clause from whichever filters are set.
func (s *Store) SearchJobs(ctx context.Context, p SearchParams) (SearchResult, error) {
	var (
		conds []string
		args  []any
	)
	// arg stores a value as a query parameter and returns its placeholder
	// ($1, $2, ...). User input only ever reaches SQL this way, never by
	// pasting it into the query string, which is what rules out SQL injection.
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	rank := ""
	if p.Query != "" {
		// websearch_to_tsquery understands what people type into search
		// boxes: `backend engineer`, `"data analyst"`, `python -django`.
		tsq := "websearch_to_tsquery('english', " + arg(p.Query) + ")"
		conds = append(conds, "search @@ "+tsq)
		rank = "ts_rank(search, " + tsq + ")"
	}
	if !p.IncludeClosed {
		conds = append(conds, "closed_at IS NULL")
	}
	if len(p.Seniority) > 0 {
		conds = append(conds, "seniority = ANY("+arg(p.Seniority)+")")
	}
	if len(p.Countries) > 0 {
		conds = append(conds, "country = ANY("+arg(p.Countries)+")")
	}
	if p.Location != "" {
		conds = append(conds, "location ILIKE "+arg("%"+escapeLike(p.Location)+"%"))
	}
	if p.Company != "" {
		conds = append(conds, "company ILIKE "+arg("%"+escapeLike(p.Company)+"%"))
	}
	if p.Remote != nil {
		conds = append(conds, "remote = "+arg(*p.Remote))
	}
	if !p.PostedSince.IsZero() {
		conds = append(conds, postedSinceCond+" >= "+arg(p.PostedSince))
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var res SearchResult
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM jobs "+where, args...).Scan(&res.Total); err != nil {
		return res, fmt.Errorf("count jobs: %w", err)
	}

	// id is the final tiebreaker so the order is fully deterministic. Without
	// it, rows with equal dates can swap between requests, and paging would
	// show some jobs twice and skip others.
	order := "posted_at DESC NULLS LAST, id DESC"
	if p.Sort == SortRelevance && rank != "" {
		order = rank + " DESC, " + order
	}

	query := fmt.Sprintf("SELECT %s FROM jobs %s ORDER BY %s LIMIT %s OFFSET %s",
		jobColumns, where, order, arg(p.Limit), arg(p.Offset))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return res, fmt.Errorf("search jobs: %w", err)
	}
	res.Jobs, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobRecord, error) {
		return scanJob(row)
	})
	if err != nil {
		return res, fmt.Errorf("scan jobs: %w", err)
	}
	return res, nil
}

type CountryCount struct {
	Code  string
	Count int
}

// postedSinceCond is a job's age for filtering: its post date, or when we
// first saw it for sources that don't give one.
const postedSinceCond = "COALESCE(posted_at, first_seen_at)"

// CountryCounts returns how many open jobs each country has, most first,
// counting only jobs posted since the given time. The search page uses it
// for its country dropdown, so the counts match what a search would return.
func (s *Store) CountryCounts(ctx context.Context, postedSince time.Time) ([]CountryCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT country, count(*) FROM jobs
		WHERE closed_at IS NULL AND country <> '' AND `+postedSinceCond+` >= $1
		GROUP BY country
		ORDER BY count(*) DESC, country`, postedSince)
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
