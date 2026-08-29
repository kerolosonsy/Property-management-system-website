package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrLastAdmin is returned when the change would leave no active administrator
// (Edge Cases). ErrSelfTarget is returned when an administrator targets their
// own account for demotion or deactivation.
var (
	ErrLastAdmin = errors.New("change would leave no active administrator")
	ErrSelfTarget = errors.New("administrator cannot target their own account")
)

// EnsureNotLastAdmin verifies the change is safe. Must run inside the same
// transaction that performs the change; the row lock over active admins makes
// the check defensible against concurrent modifications.
//
// - When demoting: targetRole must be the proposed new role. targetID must be
//   the account being changed. selfID is the caller's account id.
// - When deactivating: pass newRole == "admin", isActive=false.
//
// Returns nil when safe, ErrSelfTarget when targeting self, ErrLastAdmin when
// the change would empty the admin pool.
func EnsureNotLastAdmin(ctx context.Context, tx pgx.Tx, targetID, selfID uuid.UUID, newRole string, isActive bool) error {
	if targetID == selfID {
		return ErrSelfTarget
	}

	// Only the admin-active invariant matters when:
	//   - deactivating a current admin, or
	//   - demoting a current admin to manager.
	mustCheck := !isActive || newRole == "manager"

	if !mustCheck {
		return nil
	}

	// Lock every active admin row so a concurrent change can't slip past.
	rows, err := tx.Query(ctx, `
		SELECT id FROM account
		WHERE role = 'admin' AND is_active = true
		FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("lock admins: %w", err)
	}
	defer rows.Close()

	activeAdmins := 0
	targetWasAdmin := false
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		activeAdmins++
		if id == targetID {
			targetWasAdmin = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// If the target wasn't an active admin, the change doesn't touch the
	// active-admin count — no invariant threatened.
	if !targetWasAdmin {
		return nil
	}

	// After this change, target won't be an active admin anymore.
	if activeAdmins <= 1 {
		return ErrLastAdmin
	}
	return nil
}
