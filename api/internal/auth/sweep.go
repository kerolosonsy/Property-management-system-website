package auth

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SweepExpired removes sessions whose last_seen_at is past the idle window or
// whose created_at is past the absolute lifetime. This is housekeeping only;
// expiry is derived from the timestamps in LookupSession, so a missed sweep
// cannot leave a stale session usable (research §5 / data-model.md).
func SweepExpired(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM session
		WHERE revoked_at IS NOT NULL
		   OR last_seen_at < now() - INTERVAL '30 minutes'
		   OR created_at  < now() - INTERVAL '8 hours'`)
	if err != nil {
		slog.Error("session sweep failed", "err", err)
	}
	return err
}
