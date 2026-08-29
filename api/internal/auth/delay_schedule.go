package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// DelaySchedule is the API the auth handler uses to consult and update the
// login_failure table. The table is keyed by canonicalized submitted username
// (no FK to account), so unknown usernames accumulate delay identically
// (FR-040).
type DelaySchedule struct{}

const failureExpire = 15 * time.Minute

// Consult returns the number of seconds the caller must still wait. A row whose
// last_failure_at is older than 15 minutes is treated as fresh-from-zero, with
// a side-effect of deleting it (FR-042). Returns 0 when no wait is owed.
func (DelaySchedule) Consult(ctx context.Context, q pgx.Tx, usernameCanonical string) (int, error) {
	row := q.QueryRow(ctx,
		`SELECT consecutive_failures, last_failure_at
		 FROM login_failure WHERE username_canonical = $1`, usernameCanonical)

	var n int
	var last time.Time
	if err := row.Scan(&n, &last); err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	if time.Since(last) > failureExpire {
		_, _ = q.Exec(ctx, `DELETE FROM login_failure WHERE username_canonical = $1`, usernameCanonical)
		return 0, nil
	}
	return RemainingWait(n, last, time.Now().UTC()), nil
}

// RecordFailure increments the counter for this canonical username, creating a
// row if needed. Sweep of stale rows happens first.
func (DelaySchedule) RecordFailure(ctx context.Context, q pgx.Tx, usernameCanonical string) error {
	if _, err := q.Exec(ctx,
		`DELETE FROM login_failure WHERE last_failure_at < now() - INTERVAL '15 minutes'`,
	); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `
		INSERT INTO login_failure (username_canonical, consecutive_failures, last_failure_at)
		VALUES ($1, 1, now())
		ON CONFLICT (username_canonical)
		DO UPDATE SET consecutive_failures = login_failure.consecutive_failures + 1,
		              last_failure_at = now()
	`, usernameCanonical)
	return err
}

// Reset clears the counter for this username. Called on a successful sign-in.
func (DelaySchedule) Reset(ctx context.Context, q pgx.Tx, usernameCanonical string) error {
	_, err := q.Exec(ctx, `DELETE FROM login_failure WHERE username_canonical = $1`, usernameCanonical)
	return err
}
