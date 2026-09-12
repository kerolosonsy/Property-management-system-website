package properties

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"pms/internal/identity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// LookupKind discriminates the two identical-shape lists: property_type and
// area. It is what entity_type carries in audit rows for these entities.
type LookupKind string

const (
	LookupPropertyType LookupKind = "property_type"
	LookupArea         LookupKind = "area"
)

// ErrDuplicateLabel is returned by CreateLookup/RenameLookup when the label
// collides on the normalised unique index.
var ErrLookupDuplicateLabel = errors.New("label already in use")

// ListLookups returns every row of the named list, ordered by label.
func (s *Store) ListLookups(ctx context.Context, tx pgx.Tx, kind LookupKind) ([]Lookup, error) {
	table := lookupTable(kind)
	rows, err := tx.Query(ctx, `SELECT id, label FROM `+table+` ORDER BY label, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lookup
	for rows.Next() {
		var l Lookup
		if err := rows.Scan(&l.ID, &l.Label); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// CreateLookup inserts one row and returns it. The label is normalised in the
// application, not in a database function (research.md D-007).
func (s *Store) CreateLookup(ctx context.Context, tx pgx.Tx, kind LookupKind, label, labelNormalized string) (*Lookup, error) {
	table := lookupTable(kind)
	row := tx.QueryRow(ctx, `
		INSERT INTO `+table+` (label, label_normalized)
		VALUES ($1, $2)
		RETURNING id, label`, label, labelNormalized)
	l := &Lookup{}
	if err := row.Scan(&l.ID, &l.Label); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrLookupDuplicateLabel
		}
		return nil, err
	}
	return l, nil
}

// RenameLookup changes the label of one row, returning the row's id and the
// new label. Renaming propagates because properties refer to the entry by
// identity (data-model 0012 + research.md D-009).
func (s *Store) RenameLookup(ctx context.Context, tx pgx.Tx, kind LookupKind, id uuid.UUID, label, labelNormalized string) (*Lookup, error) {
	table := lookupTable(kind)
	row := tx.QueryRow(ctx, `
		UPDATE `+table+`
		SET label = $2, label_normalized = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, label`, id, label, labelNormalized)
	l := &Lookup{}
	if err := row.Scan(&l.ID, &l.Label); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrLookupDuplicateLabel
		}
		return nil, err
	}
	return l, nil
}

// CountLookupUsage returns the number of properties (active or archived) that
// reference this lookup id. Used to make the in-use refusal Arabic message
// state how many (FR-025). The ON DELETE RESTRICT constraint is the actual
// guarantee; this count exists only to be specific.
func (s *Store) CountLookupUsage(ctx context.Context, tx pgx.Tx, kind LookupKind, id uuid.UUID) (int, error) {
	var column string
	switch kind {
	case LookupPropertyType:
		column = "property_type_id"
	case LookupArea:
		column = "area_id"
	default:
		return 0, fmt.Errorf("unknown lookup kind %q", kind)
	}
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM property WHERE `+column+` = $1`, id).Scan(&n)
	return n, err
}

// DeleteLookup removes one row. The ON DELETE RESTRICT constraint refuses the
// delete when any property still references the row, including archived
// properties (FR-025 + research.md D-009). The caller counts first to produce
// a human-friendly Arabic message.
func (s *Store) DeleteLookup(ctx context.Context, tx pgx.Tx, kind LookupKind, id uuid.UUID) error {
	table := lookupTable(kind)
	cmd, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrInUse
		}
		return err
	}
	if cmd.RowsAffected() == 0 {
		return nil
	}
	return nil
}

// ErrInUse signals that the lookup is referenced by at least one property,
// active or archived. The store's DeleteLookup returns it when the FK
// constraint fires (research.md D-009: the count is for the message; the
// constraint is the guarantee).
var ErrInUse = errors.New("lookup in use")

// NormalizeLabel trims, collapses whitespace, and applies the same Arabic
// canonicalisation as usernames so the unique index spans letter variants.
func NormalizeLabel(raw string) string {
	return identity.Canonical(strings.TrimSpace(raw))
}

// lookupTable maps a kind to the actual table name. Centralising the
// mapping in one place keeps the SQL out of the handler bodies.
func lookupTable(k LookupKind) string {
	switch k {
	case LookupPropertyType:
		return "property_type"
	case LookupArea:
		return "area"
	default:
		return ""
	}
}
