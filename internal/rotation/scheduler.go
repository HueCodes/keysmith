// Package rotation provides scheduling utilities for the keysmith operator.
package rotation

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// NextScheduled returns the next scheduled rotation time after 'from'
// for the given standard cron expression (5-field: min hour dom month dow).
func NextScheduled(schedule string, from time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron schedule %q: %w", schedule, err)
	}
	return sched.Next(from), nil
}

// IsDue reports whether a rotation should be triggered now given:
//   - schedule: a cron expression
//   - lastRotation: the time of the most recent successful rotation (nil = never rotated)
//   - rotationWindow: rotate early if within this duration of the next scheduled time
//   - now: the current time to evaluate against
func IsDue(schedule string, lastRotation *time.Time, rotationWindow time.Duration, now time.Time) (bool, error) {
	if lastRotation == nil {
		// Never rotated: trigger immediately.
		return true, nil
	}

	next, err := NextScheduled(schedule, *lastRotation)
	if err != nil {
		return false, err
	}

	// Apply rotation window: rotate early if within the window before next scheduled time.
	effectiveNext := next.Add(-rotationWindow)
	return !now.Before(effectiveNext), nil
}

// RequeueDelay returns the duration to wait before the next reconcile
// based on the current schedule and last rotation time.
func RequeueDelay(schedule string, lastRotation *time.Time, rotationWindow time.Duration, now time.Time, minDelay time.Duration) (time.Duration, error) {
	var from time.Time
	if lastRotation != nil {
		from = *lastRotation
	} else {
		from = now
	}

	next, err := NextScheduled(schedule, from)
	if err != nil {
		return minDelay, err
	}

	effectiveNext := next.Add(-rotationWindow)
	return max(time.Until(effectiveNext), minDelay), nil
}
