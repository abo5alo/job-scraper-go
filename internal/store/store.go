// Package store is the PostgreSQL persistence layer. It uses pgx directly
// with hand-written SQL (no ORM) so every query is visible and tunable.
package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"job-scraper-go/internal/job"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// migrationLockID is an arbitrary app-wide number for pg_advisory_lock.
const migrationLockID = 72530

// Migrate applies any .sql files in migrations/ that haven't run yet, in
// filename order. Applied files are recorded in schema_migrations. This is a
// deliberately tiny version of what tools like goose or golang-migrate do.
func (s *Store) Migrate(ctx context.Context) error {
	// Advisory locks belong to a connection, so hold one connection for the
	// whole migration instead of letting the pool hand out different ones.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	// Both the scraper and the API migrate on startup. The lock makes a
	// second process wait for the first instead of running the same
	// migration twice at the same time.
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockID)

	_, err = conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		err := conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		sql, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}

		// Run the migration and record it in one transaction, so a failed
		// migration never gets marked as applied.
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name)
			return err
		})
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

// UpsertJobs inserts new postings and refreshes existing ones. On conflict we
// update the mutable fields, set last_seen_at, and reopen the job if it had
// been closed, but keep first_seen_at. All rows go in one pgx.Batch: a single
// network round trip instead of one per job.
//
// seenAt is the time the scrape run started. Using one timestamp for the
// whole run is what lets CloseMissing find the jobs this run didn't see.
func (s *Store) UpsertJobs(ctx context.Context, jobs []job.Job, seenAt time.Time) error {
	if len(jobs) == 0 {
		return nil
	}

	const q = `
		INSERT INTO jobs (
			source, external_id, board, title, company, location, country, remote,
			seniority, url, description, tags, skills, tech, salary_min, salary_max,
			salary_currency, posted_at, last_seen_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (source, external_id) DO UPDATE SET
			board           = EXCLUDED.board,
			title           = EXCLUDED.title,
			company         = EXCLUDED.company,
			location        = EXCLUDED.location,
			country         = EXCLUDED.country,
			remote          = EXCLUDED.remote,
			seniority       = EXCLUDED.seniority,
			url             = EXCLUDED.url,
			description     = EXCLUDED.description,
			tags            = EXCLUDED.tags,
			skills          = EXCLUDED.skills,
			tech            = EXCLUDED.tech,
			salary_min      = EXCLUDED.salary_min,
			salary_max      = EXCLUDED.salary_max,
			salary_currency = EXCLUDED.salary_currency,
			posted_at       = EXCLUDED.posted_at,
			last_seen_at    = EXCLUDED.last_seen_at,
			closed_at       = NULL`

	batch := &pgx.Batch{}
	for _, j := range jobs {
		batch.Queue(q,
			j.Source, j.ExternalID, j.Board, j.Title, j.Company, j.Location, j.Country, j.Remote,
			string(j.Seniority), j.URL, j.Description, notNil(j.Tags), notNil(j.Skills), j.Tech, j.SalaryMin, j.SalaryMax,
			j.SalaryCurrency, j.PostedAt, seenAt,
		)
	}

	// SendBatch results must be closed; Close also returns the first error.
	return s.pool.SendBatch(ctx, batch).Close()
}

// CloseMissing marks a board's open jobs as closed if the scrape at seenAt
// didn't include them. Only call it for boards whose feed lists every open
// job; otherwise jobs that merely fell off a "latest 100" list get closed.
func (s *Store) CloseMissing(ctx context.Context, board string, seenAt time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs SET closed_at = $2
		WHERE board = $1 AND closed_at IS NULL AND last_seen_at < $2`,
		board, seenAt)
	if err != nil {
		return 0, fmt.Errorf("close missing jobs for %s: %w", board, err)
	}
	return tag.RowsAffected(), nil
}

// CloseRemovedBoards closes the open jobs of every board that is no longer
// configured. Without it, removing a company from sources.yaml would leave
// its jobs open forever, since nothing scrapes that board anymore to notice
// they're gone.
func (s *Store) CloseRemovedBoards(ctx context.Context, configured []string, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs SET closed_at = $2
		WHERE closed_at IS NULL AND board <> ALL($1)`,
		configured, now)
	if err != nil {
		return 0, fmt.Errorf("close removed boards: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Renormalize runs job.Normalize again on every stored job and saves the
// fields it changes: level, country, company, skills and tech. Detection
// rules improve over time, and this applies a new rule to existing jobs
// without re-scraping every source. It returns how many jobs changed.
func (s *Store) Renormalize(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+jobColumns+" FROM jobs")
	if err != nil {
		return 0, fmt.Errorf("load jobs: %w", err)
	}
	jobs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobRecord, error) {
		return scanJob(row)
	})
	if err != nil {
		return 0, fmt.Errorf("load jobs: %w", err)
	}

	batch := &pgx.Batch{}
	for _, r := range jobs {
		j := r.Job
		j.Normalize()
		if j.Seniority == r.Seniority && j.Country == r.Country && j.Company == r.Company &&
			j.Tech == r.Tech && slices.Equal(j.Skills, r.Skills) {
			continue
		}
		batch.Queue(`UPDATE jobs SET seniority = $2, country = $3, company = $4, skills = $5, tech = $6 WHERE id = $1`,
			r.ID, string(j.Seniority), j.Country, j.Company, notNil(j.Skills), j.Tech)
	}
	if batch.Len() == 0 {
		return 0, nil
	}
	if err := s.pool.SendBatch(ctx, batch).Close(); err != nil {
		return 0, fmt.Errorf("save jobs: %w", err)
	}
	return batch.Len(), nil
}

// notNil turns a nil slice into an empty one. pgx sends nil as SQL NULL, and
// the array columns are NOT NULL.
func notNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
