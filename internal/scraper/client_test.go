package scraper

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testOptions() ClientOptions {
	return ClientOptions{RequestsPerSecond: 1000, MaxRetries: 3, BaseDelay: time.Millisecond, Timeout: 5 * time.Second}
}

// server replies with the given status codes in order, then 200 forever.
func server(t *testing.T, codes ...int) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(hits.Add(1))
		if n <= len(codes) {
			w.WriteHeader(codes[n-1])
			return
		}
		w.Write([]byte(`{"ok": true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestRetriesTemporaryFailures(t *testing.T) {
	srv, hits := server(t, http.StatusTooManyRequests, http.StatusServiceUnavailable)

	var body struct{ OK bool }
	if err := NewClient(testOptions()).GetJSON(context.Background(), srv.URL, &body); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if !body.OK || hits.Load() != 3 {
		t.Errorf("ok=%v hits=%d, want true after 3 attempts", body.OK, hits.Load())
	}
}

func TestDoesNotRetryPermanentFailures(t *testing.T) {
	srv, hits := server(t, http.StatusNotFound)

	_, err := NewClient(testOptions()).Get(context.Background(), srv.URL)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Fatalf("err = %v, want 404 StatusError", err)
	}
	if hits.Load() != 1 {
		t.Errorf("hits = %d, want 1 (404 won't fix itself)", hits.Load())
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	srv, hits := server(t, 500, 500, 500, 500, 500, 500)

	opts := testOptions()
	opts.MaxRetries = 2
	if _, err := NewClient(opts).Get(context.Background(), srv.URL); err == nil {
		t.Fatal("expected an error")
	}
	if hits.Load() != 3 {
		t.Errorf("hits = %d, want 3 (1 try + 2 retries)", hits.Load())
	}
}

func TestGivesUpWhenAskedToWaitTooLong(t *testing.T) {
	// Workable's response once its daily quota is used up.
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "86205")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := NewClient(testOptions()).Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected an error")
	}
	if hits.Load() != 1 {
		t.Errorf("hits = %d, want 1: asked to wait a day, we must not retry sooner", hits.Load())
	}
}

func TestRateLimitSpacesRequests(t *testing.T) {
	srv, _ := server(t)

	opts := testOptions()
	opts.RequestsPerSecond = 20 // one request every 50ms
	c := NewClient(opts)

	start := time.Now()
	for range 5 {
		resp, err := c.Get(context.Background(), srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	// First request is immediate, the next four wait ~50ms each.
	if elapsed := time.Since(start); elapsed < 180*time.Millisecond {
		t.Errorf("5 requests took %v; rate limit should space them ~50ms apart", elapsed)
	}
}

func TestParseRetryAfter(t *testing.T) {
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	tests := []struct {
		in       string
		min, max time.Duration
	}{
		{"", 0, 0},
		{"5", 5 * time.Second, 5 * time.Second},
		{"garbage", 0, 0},
		{"86205", 86205 * time.Second, 86205 * time.Second},
		{future, 28 * time.Second, 30 * time.Second},
	}
	for _, tt := range tests {
		if got := parseRetryAfter(tt.in); got < tt.min || got > tt.max {
			t.Errorf("parseRetryAfter(%q) = %v, want between %v and %v", tt.in, got, tt.min, tt.max)
		}
	}
}
