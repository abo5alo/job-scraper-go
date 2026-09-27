package store

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

type NameCount struct {
	Name  string
	Count int
}

// Stats summarizes the jobs matching a search.
type Stats struct {
	Total     int
	Levels    []NameCount // every level that has jobs, in no particular order
	Companies []NameCount // the top hirers, most jobs first
	Skills    []NameCount // the most requested skills, most jobs first
}

// Stats counts the jobs matching p by level, company and skill. The four
// queries share one filter and go to the database as one batch, so they
// cost a single round trip.
func (s *Store) Stats(ctx context.Context, p SearchParams, top int) (Stats, error) {
	f := buildFilter(p)
	// The top-N queries need one more parameter than the filter has.
	topArgs := append(slices.Clone(f.args), top)
	limit := fmt.Sprintf("LIMIT $%d", len(topArgs))

	batch := &pgx.Batch{}
	batch.Queue("SELECT count(*) FROM jobs "+f.where, f.args...)
	batch.Queue("SELECT seniority, count(*) FROM jobs "+f.where+" GROUP BY seniority", f.args...)
	// HAVING drops jobs whose employer is hidden (company "").
	batch.Queue("SELECT company, count(*) FROM jobs "+f.where+
		" GROUP BY company HAVING company <> '' ORDER BY count(*) DESC, company "+limit, topArgs...)
	// unnest turns each job's skills array into one row per skill, so
	// GROUP BY can count how many jobs mention each.
	batch.Queue("SELECT skill, count(*) FROM jobs CROSS JOIN LATERAL unnest(skills) AS skill "+f.where+
		" GROUP BY skill ORDER BY count(*) DESC, skill "+limit, topArgs...)

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	var st Stats
	if err := br.QueryRow().Scan(&st.Total); err != nil {
		return st, fmt.Errorf("count jobs: %w", err)
	}
	for _, dst := range []*[]NameCount{&st.Levels, &st.Companies, &st.Skills} {
		rows, err := br.Query()
		if err != nil {
			return st, fmt.Errorf("stats: %w", err)
		}
		if *dst, err = pgx.CollectRows(rows, pgx.RowToStructByPos[NameCount]); err != nil {
			return st, fmt.Errorf("stats: %w", err)
		}
	}
	return st, br.Close()
}
