package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Account is a row in the account table. It carries the typed form (Username)
// and the canonical form (UsernameCanonical). Matching uses the canonical form;
// display uses the typed form (FR-039).
type Account struct {
	ID                uuid.UUID
	Username          string
	UsernameCanonical string
	DisplayName       string
	PasswordHash      string
	Role              string // "admin" or "manager"
	IsActive          bool
	MustChangePassword bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CreatedBy         *uuid.UUID
}

type Store struct {
	Pool *pgxpool.Pool
}

func (s *Store) Pool_() *pgxpool.Pool { return s.Pool }

// CreateAccount inserts a new account. Returning ErrDuplicateUsername means the
// canonical username already exists (FR-038).
func (s *Store) CreateAccount(ctx context.Context, tx pgx.Tx, a *Account, passwordHash string, createdBy *uuid.UUID) error {
	const q = `
		INSERT INTO account (username, username_canonical, display_name, password_hash,
		                     role, is_active, must_change_password, created_by)
		VALUES ($1, $2, $3, $4, $5, true, true, $6)
		RETURNING id, created_at, updated_at, must_change_password, is_active`
	row := tx.QueryRow(ctx, q,
		a.Username, a.UsernameCanonical, a.DisplayName, passwordHash, a.Role, createdBy)
	if err := row.Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt, &a.MustChangePassword, &a.IsActive); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrDuplicateUsername
		}
		return err
	}
	a.PasswordHash = passwordHash
	return nil
}

// FindByCanonical returns the account whose canonical username matches, or
// (nil, nil) if none. The match is canonical so letter case and Arabic letter
// variants do not create a second account (FR-038).
func (s *Store) FindByCanonical(ctx context.Context, q pgx.Tx, canonical string) (*Account, error) {
	const sql = `
		SELECT id, username, username_canonical, display_name, password_hash,
		       role, is_active, must_change_password, created_at, updated_at, created_by
		FROM account WHERE username_canonical = $1`
	row := q.QueryRow(ctx, sql, canonical)
	a := &Account{}
	if err := row.Scan(&a.ID, &a.Username, &a.UsernameCanonical, &a.DisplayName, &a.PasswordHash,
		&a.Role, &a.IsActive, &a.MustChangePassword, &a.CreatedAt, &a.UpdatedAt, &a.CreatedBy); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// FindByID looks up by UUID.
func (s *Store) FindByID(ctx context.Context, q pgx.Tx, id uuid.UUID) (*Account, error) {
	const sql = `
		SELECT id, username, username_canonical, display_name, password_hash,
		       role, is_active, must_change_password, created_at, updated_at, created_by
		FROM account WHERE id = $1`
	row := q.QueryRow(ctx, sql, id)
	a := &Account{}
	if err := row.Scan(&a.ID, &a.Username, &a.UsernameCanonical, &a.DisplayName, &a.PasswordHash,
		&a.Role, &a.IsActive, &a.MustChangePassword, &a.CreatedAt, &a.UpdatedAt, &a.CreatedBy); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// FindInitialAdmin returns the very first administrator (the one with
// created_by IS NULL), if any. Used at install time.
func (s *Store) FindInitialAdmin(ctx context.Context, q pgx.Tx) (*Account, error) {
	const sql = `
		SELECT id, username, username_canonical, display_name, password_hash,
		       role, is_active, must_change_password, created_at, updated_at, created_by
		FROM account WHERE created_by IS NULL LIMIT 1`
	row := q.QueryRow(ctx, sql)
	a := &Account{}
	if err := row.Scan(&a.ID, &a.Username, &a.UsernameCanonical, &a.DisplayName, &a.PasswordHash,
		&a.Role, &a.IsActive, &a.MustChangePassword, &a.CreatedAt, &a.UpdatedAt, &a.CreatedBy); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// UpdatePassword writes a new password hash and clears must_change_password.
func (s *Store) UpdatePassword(ctx context.Context, q pgx.Tx, id uuid.UUID, passwordHash string, mustChange bool) error {
	_, err := q.Exec(ctx, `
		UPDATE account SET password_hash = $2, must_change_password = $3, updated_at = now()
		WHERE id = $1`, id, passwordHash, mustChange)
	return err
}

// UpdateAccount applies display_name, role, is_active changes. Pass nil for
// any field to leave it untouched.
func (s *Store) UpdateAccount(ctx context.Context, q pgx.Tx, id uuid.UUID, displayName *string, role *string, isActive *bool) error {
	parts := []string{"updated_at = now()"}
	args := []any{id}
	idx := 2
	if displayName != nil {
		parts = append(parts, "display_name = $"+itoa(idx))
		args = append(args, *displayName)
		idx++
	}
	if role != nil {
		parts = append(parts, "role = $"+itoa(idx))
		args = append(args, *role)
		idx++
	}
	if isActive != nil {
		parts = append(parts, "is_active = $"+itoa(idx))
		args = append(args, *isActive)
		idx++
	}
	sql := "UPDATE account SET " + strings.Join(parts, ", ") + " WHERE id = $1"
	_, err := q.Exec(ctx, sql, args...)
	return err
}

// ListAccounts returns accounts newest first, with paging. includeInactive
// toggles whether deactivated accounts are returned.
func (s *Store) ListAccounts(ctx context.Context, q pgx.Tx, includeInactive bool, page, pageSize int) ([]Account, int, error) {
	offset := (page - 1) * pageSize
	whereClause := ""
	if !includeInactive {
		whereClause = "WHERE is_active"
	}
	rows, err := q.Query(ctx, `
		SELECT id, username, username_canonical, display_name, password_hash,
		       role, is_active, must_change_password, created_at, updated_at, created_by
		FROM account `+whereClause+`
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Username, &a.UsernameCanonical, &a.DisplayName, &a.PasswordHash,
			&a.Role, &a.IsActive, &a.MustChangePassword, &a.CreatedAt, &a.UpdatedAt, &a.CreatedBy); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM account `+whereClause).Scan(&total); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

var ErrDuplicateUsername = errors.New("username already in use")

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
