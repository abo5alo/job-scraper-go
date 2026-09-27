// Package api serves jobs over HTTP using only the standard library router.
//
//	GET /            the search page
//	GET /jobs        search and filter open jobs
//	GET /jobs/{id}   one job, with its full description
//	GET /stats       levels, top companies and top skills for a search
//	GET /countries   open job counts per country
//	GET /healthz     liveness check, including the database
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"job-scraper-go/internal/store"
	"job-scraper-go/internal/web"
)

// JobStore is the part of the store the API uses. The interface is defined
// here, where it's consumed, so handler tests can pass a fake instead of
// needing a real database.
type JobStore interface {
	SearchJobs(ctx context.Context, p store.SearchParams) (store.SearchResult, error)
	GetJob(ctx context.Context, id int64) (store.JobRecord, error)
	Stats(ctx context.Context, p store.SearchParams, top int) (store.Stats, error)
	CountryCounts(ctx context.Context, postedSince time.Time, techOnly bool) ([]store.CountryCount, error)
	Ping(ctx context.Context) error
}

type Options struct {
	// Per-client-IP rate limit for the whole API.
	RequestsPerSecond float64
	Burst             int
}

// maxJobAge hides postings older than about 3 months from search. Some
// companies never take old postings down (Namshi still lists jobs from 2017),
// and a years-old "open" job is almost never really hiring. The jobs stay in
// the database for history and analytics; they just aren't shown.
const maxJobAge = 90 * 24 * time.Hour

func oldestPostDate() time.Time {
	return time.Now().Add(-maxJobAge)
}

type server struct {
	store JobStore
	log   *slog.Logger
}

// NewHandler wires the routes and middleware. Middleware wraps from the
// outside in: logging sees every request, including ones the rate limiter
// rejects.
func NewHandler(st JobStore, log *slog.Logger, opts Options) http.Handler {
	s := &server{store: st, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /jobs", s.searchJobs)
	mux.HandleFunc("GET /jobs/{id}", s.getJob)
	mux.HandleFunc("GET /stats", s.stats)
	mux.HandleFunc("GET /countries", s.countries)
	mux.HandleFunc("GET /healthz", s.health)
	// Everything else is the search page and its files.
	mux.Handle("GET /", web.Handler())

	limited := newIPRateLimiter(opts.RequestsPerSecond, opts.Burst).middleware(mux)
	return logRequests(log, limited)
}

func (s *server) searchJobs(w http.ResponseWriter, r *http.Request) {
	params, page, err := parseSearchParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	params.PostedSince = oldestPostDate()

	res, err := s.store.SearchJobs(r.Context(), params)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	resp := searchResponse{
		Total: res.Total,
		Page:  page,
		Limit: params.Limit,
		Jobs:  make([]jobResponse, 0, len(res.Jobs)), // [] not null when empty
	}
	for _, j := range res.Jobs {
		resp.Jobs = append(resp.Jobs, toJobResponse(j, listDescriptionLen))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) getJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	j, err := s.store.GetJob(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toJobResponse(j, 0))
}

// statsTop is how many companies and skills /stats returns.
const statsTop = 15

// stats takes the same filters as /jobs and describes the matching jobs as a
// whole: how they split by level, who's hiring most, and which skills they
// ask for. Paging and sort params are accepted but don't apply.
func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	params, _, err := parseSearchParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	params.PostedSince = oldestPostDate()

	st, err := s.store.Stats(r.Context(), params, statsTop)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toStatsResponse(st))
}

func (s *server) countries(w http.ResponseWriter, r *http.Request) {
	params, _, err := parseSearchParams(url.Values{"tech": {r.URL.Query().Get("tech")}})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	techOnly := params.Tech != nil && *params.Tech

	counts, err := s.store.CountryCounts(r.Context(), oldestPostDate(), techOnly)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	type countryResponse struct {
		Code  string `json:"code"`
		Count int    `json:"count"`
	}
	resp := make([]countryResponse, 0, len(counts))
	for _, c := range counts {
		resp = append(resp, countryResponse{c.Code, c.Count})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		s.log.Error("health check failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// internalError logs the real error but tells the client nothing about it:
// database errors can leak table names, queries or connection details.
func (s *server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
