package rotation_test

import (
	"testing"
	"time"

	"github.com/hstores/keysmith/internal/rotation"
)

func TestNextScheduled_DailyAtMidnight(t *testing.T) {
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	next, err := rotation.NextScheduled("0 0 * * *", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestNextScheduled_InvalidCron(t *testing.T) {
	_, err := rotation.NextScheduled("not-a-cron", time.Now())
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestIsDue_NeverRotated(t *testing.T) {
	due, err := rotation.IsDue("0 0 * * *", nil, 0, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !due {
		t.Error("expected due=true for a policy that has never been rotated")
	}
}

func TestIsDue_JustRotated(t *testing.T) {
	lastRotation := time.Now().Add(-1 * time.Minute)
	due, err := rotation.IsDue("0 0 * * *", &lastRotation, 0, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if due {
		t.Error("expected due=false for a policy rotated 1 minute ago on a daily schedule")
	}
}

func TestIsDue_OverdueRotation(t *testing.T) {
	lastRotation := time.Now().Add(-25 * time.Hour)
	due, err := rotation.IsDue("0 0 * * *", &lastRotation, 0, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !due {
		t.Error("expected due=true for a policy last rotated 25 hours ago on a daily schedule")
	}
}

func TestIsDue_WithinRotationWindow(t *testing.T) {
	// Last rotation was 23h ago on an hourly schedule.
	// Next is in 1h. With a 2h window, should be due early.
	lastRotation := time.Now().Add(-23 * time.Hour)
	due, err := rotation.IsDue("0 * * * *", &lastRotation, 2*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !due {
		t.Error("expected due=true: within rotation window before next scheduled time")
	}
}

func TestIsDue_InvalidCron(t *testing.T) {
	lastRotation := time.Now().Add(-1 * time.Hour)
	_, err := rotation.IsDue("bad-cron", &lastRotation, 0, time.Now())
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestRequeueDelay_ReturnsPositive(t *testing.T) {
	lastRotation := time.Now()
	delay, err := rotation.RequeueDelay("0 0 * * *", &lastRotation, 0, time.Now(), 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delay <= 0 {
		t.Errorf("expected positive requeue delay, got %v", delay)
	}
}

func TestRequeueDelay_RespectsMinDelay(t *testing.T) {
	lastRotation := time.Now().Add(-25 * time.Hour)
	minDelay := 30 * time.Second
	delay, err := rotation.RequeueDelay("0 0 * * *", &lastRotation, 0, time.Now(), minDelay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delay < minDelay {
		t.Errorf("expected delay >= minDelay (%v), got %v", minDelay, delay)
	}
}
