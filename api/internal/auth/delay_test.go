package auth

import (
	"testing"
	"time"
)

func TestRequiredWaitProgression(t *testing.T) {
	want := []int{1, 2, 4, 8, 16, 32, 60, 60, 60}
	for i, w := range want {
		if got := RequiredWait(i + 1); got != w {
			t.Errorf("RequiredWait(%d) = %d; want %d", i+1, got, w)
		}
	}
}

func TestRequiredWaitZero(t *testing.T) {
	if got := RequiredWait(0); got != 0 {
		t.Errorf("RequiredWait(0) = %d; want 0", got)
	}
}

func TestRemainingWaitBefore(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-1 * time.Second) // 1 failure means 1s wait; 1s elapsed → 0 remaining
	if got := RemainingWait(1, last, now); got != 0 {
		t.Errorf("RemainingWait = %d; want 0", got)
	}
}

func TestRemainingWaitStillRunning(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now // 1 failure = 1s wait; 0 elapsed → 1s remaining
	if got := RemainingWait(1, last, now); got != 1 {
		t.Errorf("RemainingWait = %d; want 1", got)
	}
}

func TestRemainingWaitLarge(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now // 5 failures = 16s wait
	if got := RemainingWait(5, last, now); got != 16 {
		t.Errorf("RemainingWait(5, now, now) = %d; want 16", got)
	}
}

func TestRemainingWaitClamped(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now // 100 failures = 60s wait (clamped)
	if got := RemainingWait(100, last, now); got != 60 {
		t.Errorf("RemainingWait(100, now, now) = %d; want 60", got)
	}
}

func TestHashAndVerify(t *testing.T) {
	pw := "الإدارة2026#قوي"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := VerifyPassword(pw, h); err != nil {
		t.Errorf("verify same pw failed: %v", err)
	}
	if err := VerifyPassword(pw+"x", h); err == nil {
		t.Errorf("verify wrong pw should fail")
	}
}
