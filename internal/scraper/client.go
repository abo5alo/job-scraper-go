package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Client is the HTTP client every scraper shares. On top of http.Client it
// adds the manners a well-behaved scraper needs:
//
//   - A rate limit per host. Ten companies on Greenhouse means ten requests to
//     the same server; they get spaced out instead of sent in a burst.
//   - Retries with exponential backoff for failures that are likely temporary
//     (network errors, 429 Too Many Requests, 5xx server errors), but not for
//     ones that won't fix themselves (404, 403).
//   - Respect for the server's Retry-After header when it tells us how long
//     to back off.
type Client struct {
	http *http.Client
	opts ClientOptions

	// Many scraper goroutines share one Client, so the limiters map is
	// guarded by a mutex. Each limiter is itself safe for concurrent use.
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

type ClientOptions struct {
	// RequestsPerSecond is the limit for each host, not overall. Scraping two
	// different sites at once is fine; hammering one site is not.
	RequestsPerSecond float64
	// MaxRetries is how many times a failed request is retried (0 = never).
	MaxRetries int
	// BaseDelay is the wait before the first retry. It doubles each retry.
	BaseDelay time.Duration
	// Timeout bounds a single request attempt.
	Timeout time.Duration
}

func DefaultClientOptions() ClientOptions {
	return ClientOptions{
		RequestsPerSecond: 2,
		MaxRetries:        3,
		BaseDelay:         time.Second,
		Timeout:           30 * time.Second,
	}
}

const (
	maxBackoff = 30 * time.Second
	// maxRetryAfter is the longest Retry-After we'll wait out. Anything
	// longer fails the request instead: parking a scraper for hours helps
	// no one, and retrying sooner than asked is rude.
	maxRetryAfter = time.Minute
)

func NewClient(opts ClientOptions) *Client {
	return &Client{
		http:     &http.Client{Timeout: opts.Timeout},
		opts:     opts,
		limiters: make(map[string]*rate.Limiter),
	}
}

// StatusError is returned when the server's final answer is not 2xx.
type StatusError struct {
	URL  string
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: %d %s", e.URL, e.Code, http.StatusText(e.Code))
}

// GetJSON fetches url and decodes the JSON response body into v.
func (c *Client) GetJSON(ctx context.Context, url string, v any) error {
	resp, err := c.Get(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// Get fetches url, waiting its turn on the host's rate limiter and retrying
// temporary failures. The caller must close the response body.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)

	limiter := c.limiterFor(req.URL.Host)

	for attempt := 0; ; attempt++ {
		// Retries go through the limiter too, so retrying can't turn into
		// the burst of requests the limiter exists to prevent.
		if err := limiter.Wait(ctx); err != nil {
			return nil, err
		}

		resp, err := c.http.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		var retryAfter time.Duration
		if err == nil {
			retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
			// Drain and close the body so the TCP connection can be reused.
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			err = &StatusError{URL: url, Code: resp.StatusCode}
		}

		if ctx.Err() != nil {
			return nil, ctx.Err() // cancelled or timed out: don't retry
		}
		if !retryable(err) || attempt >= c.opts.MaxRetries {
			return nil, err
		}

		// A server asking us to wait longer than we're willing to (Workable
		// answers a used-up daily quota with "Retry-After: 86205", about 24
		// hours) means stop, not "retry a bit sooner than asked".
		if retryAfter > maxRetryAfter {
			return nil, fmt.Errorf("%w; server asked us to wait %v, giving up", err, retryAfter.Round(time.Minute))
		}

		delay := retryAfter
		if delay == 0 {
			delay = c.backoff(attempt)
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *Client) limiterFor(host string) *rate.Limiter {
	c.mu.Lock()
	defer c.mu.Unlock()

	l, ok := c.limiters[host]
	if !ok {
		// A token bucket that refills at RequestsPerSecond. Burst 1 means
		// requests are always evenly spaced, never sent in a clump.
		l = rate.NewLimiter(rate.Limit(c.opts.RequestsPerSecond), 1)
		c.limiters[host] = l
	}
	return l
}

// retryable reports whether a failed request might succeed if tried again.
func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code == http.StatusTooManyRequests || se.Code >= 500
	}
	return true // network-level errors: connection reset, timeout, DNS blip
}

// backoff returns the delay before retry number attempt (0-based): the base
// delay doubled per attempt, with random jitter. Jitter matters when many
// clients fail at once. Without it they all retry at the same instant and
// overload the recovering server again.
func (c *Client) backoff(attempt int) time.Duration {
	d := min(c.opts.BaseDelay<<attempt, maxBackoff)
	if d <= 0 {
		return 0
	}
	// Somewhere between half and all of d.
	return d/2 + rand.N(d/2+1)
}

// parseRetryAfter reads a Retry-After header, which is either a number of
// seconds ("120") or an HTTP date. It returns 0 if the header is missing or
// invalid.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.Atoi(v); err == nil {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = time.Until(t)
	}
	return max(d, 0)
}
