package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParseTimeOfDay(t *testing.T) {
	if d, err := parseTimeOfDay("03:30"); err != nil || d != (timeOfDay{3, 30}) {
		t.Errorf("parseTimeOfDay(03:30) = %+v, %v", d, err)
	}
	for _, bad := range []string{"", "3pm", "25:00", "03:60", "03"} {
		if _, err := parseTimeOfDay(bad); err == nil {
			t.Errorf("parseTimeOfDay(%q) succeeded, want an error", bad)
		}
	}
}

func TestTimeOfDayNext(t *testing.T) {
	at := timeOfDay{3, 0}
	utc := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	tests := []struct{ now, want string }{
		{"2026-10-01T01:00:00Z", "2026-10-01T03:00:00Z"}, // later today
		{"2026-10-01T03:00:00Z", "2026-10-02T03:00:00Z"}, // exactly now: the next day, so a run never repeats
		{"2026-10-01T22:00:00Z", "2026-10-02T03:00:00Z"}, // tomorrow
		{"2026-10-31T12:00:00Z", "2026-11-01T03:00:00Z"}, // across a month end
		// 01:00 in Riyadh (UTC+3) is 22:00 UTC the day before: still UTC's 03:00.
		{"2026-10-02T01:00:00+03:00", "2026-10-02T03:00:00Z"},
	}
	for _, tt := range tests {
		if got := at.next(utc(tt.now)); !got.Equal(utc(tt.want)) {
			t.Errorf("next(%s) = %s, want %s", tt.now, got.Format(time.RFC3339), tt.want)
		}
	}
}

func TestSleepUntil(t *testing.T) {
	if err := sleepUntil(context.Background(), time.Now().Add(-time.Second)); err != nil {
		t.Errorf("past time: %v, want nil right away", err)
	}

	start := time.Now()
	if err := sleepUntil(context.Background(), start.Add(50*time.Millisecond)); err != nil || time.Since(start) < 50*time.Millisecond {
		t.Errorf("returned after %v with %v, want nil after 50ms", time.Since(start), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepUntil(ctx, time.Now().Add(time.Hour)); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v, want context.Canceled", err)
	}
}
