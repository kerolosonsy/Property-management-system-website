package properties

import (
	"context"
	"errors"
	"fmt"

	"pms/internal/identity"

	"github.com/jackc/pgx/v5"
)

// Reference code shape:
//   P-<NNN>   generated form, three-digit zero-padded
//   <custom>  an administrator-supplied override
//
// The numeric part comes from the property_code_seq sequence; the override
// path lets an administrator supply any 1–32 character code provided no row
// already holds it. research.md D-001 calls out the collision: an override of
// "P-005" while the sequence still sits at 3 makes the next generated insert
// race the override. The bounded retry handles exactly that case.

// maxGenerateAttempts is the budget for the sequence-loop. Five attempts is
// enough that only a deliberate adversary can exhaust it, per research.md
// D-001.
const maxGenerateAttempts = 5

// codeSequence is the database sequence nextval call, lifted out so the retry
// logic can be exercised without a database.
type codeSequence func(ctx context.Context) (int64, error)

// codeExists reports whether a normalized code is already held by some row,
// active or archived (data-model 0012: the unique index spans archived rows).
type codeExists func(ctx context.Context, codeNormalized string) (bool, error)

// GenerateCode returns the next reference code using property_code_seq. The
// caller holds a transaction; this method only reads the sequence.
func (s *Store) GenerateCode(ctx context.Context, tx pgx.Tx) (string, error) {
	return generateCode(ctx,
		func(c context.Context) (int64, error) {
			var n int64
			if err := tx.QueryRow(c, `SELECT nextval('property_code_seq')`).Scan(&n); err != nil {
				return 0, fmt.Errorf("read sequence: %w", err)
			}
			return n, nil
		},
		func(c context.Context, codeNormalized string) (bool, error) {
			return s.CodeExists(c, tx, codeNormalized)
		},
	)
}

// generateCode is the pure retry loop. Sequence and exists are injected so
// tests can cover the collision and exhaustion branches without a database.
func generateCode(ctx context.Context, seq codeSequence, exists codeExists) (string, error) {
	for attempt := 0; attempt < maxGenerateAttempts; attempt++ {
		n, err := seq(ctx)
		if err != nil {
			return "", err
		}
		code := fmt.Sprintf("P-%03d", n)
		taken, err := exists(ctx, identity.Canonical(code))
		if err != nil {
			return "", err
		}
		if !taken {
			return code, nil
		}
		// A collision means an administrator set a code whose numeric part
		// we are about to assign. Take the next value and try again.
	}
	return "", errors.New("exhausted reference-code retry budget")
}

// VerifyCodeIsAvailable reports whether the supplied code is free. "Free"
// means no row — active or archived — currently holds it (data-model 0012).
// The second return value distinguishes "this code belongs to an archived
// property" so the caller can produce a more specific Arabic message.
func (s *Store) VerifyCodeIsAvailable(ctx context.Context, tx pgx.Tx, codeNormalized string) (free bool, archived bool, err error) {
	var archivedAt *string
	row := tx.QueryRow(ctx, `SELECT archived_at::text FROM property WHERE code_normalized = $1`, codeNormalized)
	if err := row.Scan(&archivedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, false, nil
		}
		return false, false, err
	}
	return false, archivedAt != nil, nil
}
