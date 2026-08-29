package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Session represents one row in the session table. The token is never stored;
// only its SHA-256 is (research §5).
type Session struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	TokenSHA256 []byte
	CreatedAt   time.Time
	LastSeenAt  time.Time
	RevokedAt   *time.Time
}

func newOpaqueToken() (raw string, hash []byte, err error) {
	buf := make([]byte, 32) // 256-bit
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}

// IssueSession creates a new session row for the account and returns the raw
// token to put in the cookie. The database stores only the SHA-256.
func IssueSession(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) (raw string, err error) {
	raw, hash, err := newOpaqueToken()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO session (account_id, token_sha256, created_at, last_seen_at)
		 VALUES ($1, $2, now(), now())`,
		accountID, hash)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return raw, nil
}

// LookupSession returns the session row for the raw token, or (nil, nil) if no
// live session exists. "Live" means not revoked AND within the idle and absolute
// limits (FR-017, FR-018). The caller is responsible for updating last_seen_at
// (TouchSession).
func LookupSession(ctx context.Context, q pgx.Tx, raw string) (*Session, error) {
	sum := sha256.Sum256([]byte(raw))
	row := q.QueryRow(ctx, `
		SELECT id, account_id, token_sha256, created_at, last_seen_at, revoked_at
		FROM session
		WHERE token_sha256 = $1`, sum[:])

	var s Session
	if err := row.Scan(&s.ID, &s.AccountID, &s.TokenSHA256, &s.CreatedAt, &s.LastSeenAt, &s.RevokedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if s.RevokedAt != nil {
		return nil, nil
	}
	now := time.Now().UTC()
	if now.Sub(s.LastSeenAt) > 30*time.Minute {
		return nil, nil
	}
	if now.Sub(s.CreatedAt) > 8*time.Hour {
		return nil, nil
	}
	return &s, nil
}

// TouchSession updates last_seen_at = now() so a request right before the idle
// boundary keeps the session alive (FR-017).
func TouchSession(ctx context.Context, q pgx.Tx, id uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE session SET last_seen_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	return err
}

// RevokeSession marks the session revoked. Called by sign-out.
func RevokeSession(ctx context.Context, q pgx.Tx, id uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE session SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	return err
}

// RevokeAllForAccount revokes every live session for the account. Called by
// deactivation, password reset, and password change (FR-020).
func RevokeAllForAccount(ctx context.Context, q pgx.Tx, accountID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE session SET revoked_at = now() WHERE account_id = $1 AND revoked_at IS NULL`, accountID)
	return err
}
