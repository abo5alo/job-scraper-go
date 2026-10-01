package main

import (
	"context"
	"fmt"
	"time"
)

// timeOfDay is when the daily scrape runs, in UTC. A fixed time rather than
// "every 24 hours from start" keeps runs a day apart even when the process
// restarts, which matters for sources with a daily request quota.
type timeOfDay struct {
	hour, minute int
}

// parseTimeOfDay reads a 24-hour "HH:MM" time like "03:00".
func parseTimeOfDay(s string) (timeOfDay, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return timeOfDay{}, fmt.Errorf("daily-at %q: want a 24-hour time like 03:00", s)
	}
	return timeOfDay{t.Hour(), t.Minute()}, nil
}

// next returns the first time strictly after now that falls on this time of
// day, in UTC.
func (d timeOfDay) next(now time.Time) time.Time {
	now = now.UTC()
	t := time.Date(now.Year(), now.Month(), now.Day(), d.hour, d.minute, 0, 0, time.UTC)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// maxSleep is the longest sleepUntil waits before checking the clock again.
const maxSleep = time.Minute

// sleepUntil waits until the wall clock reaches t, or returns ctx's error if
// ctx is cancelled first.
//
// It wakes at least once a minute to check the clock instead of sleeping
// once for the whole wait. Timers measure elapsed time, and a computer that
// sleeps overnight doesn't count the hours it spent asleep: a single 20-hour
// timer could fire many hours late. Rechecking the wall clock means the run
// starts within a minute of the computer waking up.
func sleepUntil(ctx context.Context, t time.Time) error {
	for {
		wait := time.Until(t)
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(min(wait, maxSleep))
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}
