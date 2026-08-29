package auth

import (
	"math"
	"time"
)

// RequiredWait returns the number of seconds the caller must wait before another
// sign-in attempt is accepted, given n consecutive failures and the time of the
// most recent failure. The schedule is min(60, 2^(n-1)) seconds, so 1, 2, 4, 8,
// 16, 32, 60, 60… (FR-014). n=0 returns 0.
func RequiredWait(n int) int {
	if n <= 0 {
		return 0
	}
	shift := n - 1
	if shift > 30 {
		shift = 30
	}
	w := 1 << shift
	if w > 60 {
		w = 60
	}
	return w
}

// RemainingWait returns the seconds the caller must STILL wait, given the
// latest failure count and the time of the most recent failure. Returns 0 when
// the wait has passed. The result is clamped to [0, 60].
func RemainingWait(n int, lastFailure time.Time, now time.Time) int {
	if n <= 0 {
		return 0
	}
	req := time.Duration(RequiredWait(n)) * time.Second
	elapsed := now.Sub(lastFailure)
	remaining := req - elapsed
	if remaining <= 0 {
		return 0
	}
	secs := int(math.Ceil(remaining.Seconds()))
	if secs > 60 {
		secs = 60
	}
	return secs
}
