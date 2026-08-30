// Package audit writes the append-only audit_log rows and serves the records
// screen. Every write happens inside the caller's transaction, so the audit
// row and the change it records commit or roll back together (Constitution
// VIII).
package audit

import (
	"context"
	"encoding/json"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Action string

const (
	SignInSucceeded     Action = "sign_in_succeeded"
	SignInFailed        Action = "sign_in_failed"
	SignOut             Action = "sign_out"
	PasswordChanged     Action = "password_changed"
	PasswordReset       Action = "password_reset"
	AccountCreated      Action = "account_created"
	AccountRoleChanged  Action = "account_role_changed"
	AccountActivated    Action = "account_activated"
	AccountDeactivated  Action = "account_deactivated"
	SessionsInvalidated Action = "sessions_invalidated"
	AdminRecoveryUsed   Action = "admin_recovery_used"

	PropertyCreated           Action = "property_created"
	PropertyModified          Action = "property_modified"
	PropertyArchived          Action = "property_archived"
	PropertyRestored          Action = "property_restored"
	PropertyCodeChanged       Action = "property_code_changed"
	LookupCreated             Action = "lookup_created"
	LookupRenamed             Action = "lookup_renamed"
	LookupRemoved             Action = "lookup_removed"
	CustomFieldCreated        Action = "custom_field_created"
	CustomFieldRenamed        Action = "custom_field_renamed"
	CustomFieldRemoved        Action = "custom_field_removed"
	CustomFieldChoiceAdded    Action = "custom_field_choice_added"
	CustomFieldChoiceRemoved  Action = "custom_field_choice_removed"
)

// EntityType names the business record a row concerns. The values mirror the
// audit_entity enum created by migration 0009.
type EntityType string

const (
	EntityProperty     EntityType = "property"
	EntityPropertyType EntityType = "property_type"
	EntityArea         EntityType = "area"
	EntityCustomField  EntityType = "custom_field"
)

type Entry struct {
	Action          Action
	ActorAccountID  *uuid.UUID
	ActorUsername   string
	ActorRole       *string
	EntityType      EntityType
	EntityID        string
	TargetAccountID *uuid.UUID
	TargetUsername  *string
	SourceIP        string
	Detail          any // JSON-serialisable; identifiers and field names only
}

// Write appends the entry to audit_log. The caller's pgx.Tx commits or rolls
// back the change together with the audit row. Constitution VIII: an audit
// failure must roll back the change it records.
//
// Callers MUST check the returned error and MUST NOT call tx.Commit when it is
// non-nil — the underlying audit row is the only durable proof that the change
// happened, so a failed write must take the change with it. Discarding the
// error (the historical `_ = audit.Write(...)` pattern) leaves the change
// committed with no audit trail and violates FR-029.
func Write(ctx context.Context, tx pgx.Tx, e Entry) error {
	var detailJSON []byte
	if e.Detail != nil {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return err
		}
		detailJSON = b
	}
	var actorRole any
	if e.ActorRole != nil {
		actorRole = *e.ActorRole
	}
	var entityType any
	if e.EntityType != "" {
		entityType = string(e.EntityType)
	}
	var entityID any
	if e.EntityID != "" {
		entityID = e.EntityID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_log (
			occurred_at, action,
			actor_account_id, actor_username_snapshot, actor_role,
			entity_type, entity_id,
			target_account_id, target_username_snapshot,
			source_ip, detail
		) VALUES (
			now(), $1,
			$2, $3, $4,
			$5, $6,
			$7, $8,
			$9, $10
		)`,
		string(e.Action),
		e.ActorAccountID, e.ActorUsername, actorRole,
		entityType, entityID,
		e.TargetAccountID, e.TargetUsername,
		e.SourceIP, detailJSON,
	)
	return err
}

type Record struct {
	ID              int64
	OccurredAt      time.Time
	Action          Action
	ActorID         *uuid.UUID
	ActorUsername   string
	ActorRole       *string
	EntityType      *EntityType
	EntityID        *string
	TargetID        *uuid.UUID
	TargetUsername  *string
	SourceIP        netip.Addr
	Detail          []byte
}

type Filters struct {
	ActorID  *uuid.UUID
	TargetID *uuid.UUID
	Action   *Action
	From     *time.Time // inclusive start (UTC)
	To       *time.Time // inclusive end (UTC); date-only ranges are expanded to a full day by the caller
	Page     int
	PageSize int
}

// Query returns a page of records newest first, plus the total count.
func Query(ctx context.Context, pool *pgxpool.Pool, f Filters) ([]Record, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 25
	}
	where := []string{"1=1"}
	args := []any{}
	idx := 1
	if f.ActorID != nil {
		where = append(where, "actor_account_id = $"+itoa(idx))
		args = append(args, *f.ActorID)
		idx++
	}
	if f.TargetID != nil {
		where = append(where, "target_account_id = $"+itoa(idx))
		args = append(args, *f.TargetID)
		idx++
	}
	if f.Action != nil {
		where = append(where, "action = $"+itoa(idx))
		args = append(args, string(*f.Action))
		idx++
	}
	if f.From != nil {
		where = append(where, "occurred_at >= $"+itoa(idx))
		args = append(args, *f.From)
		idx++
	}
	if f.To != nil {
		where = append(where, "occurred_at <= $"+itoa(idx))
		args = append(args, *f.To)
		idx++
	}
	whereSQL := joinAnd(where)

	var total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := pool.Query(ctx, `
		SELECT id, occurred_at, action,
		       actor_account_id, actor_username_snapshot, actor_role,
		       entity_type, entity_id,
		       target_account_id, target_username_snapshot,
		       source_ip, detail
		FROM audit_log WHERE `+whereSQL+`
		ORDER BY occurred_at DESC, id DESC
		LIMIT $`+itoa(idx)+` OFFSET $`+itoa(idx+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var detail []byte
		if err := rows.Scan(&r.ID, &r.OccurredAt, &r.Action,
			&r.ActorID, &r.ActorUsername, &r.ActorRole,
			&r.EntityType, &r.EntityID,
			&r.TargetID, &r.TargetUsername,
			&r.SourceIP, &detail); err != nil {
			return nil, 0, err
		}
		r.Detail = detail
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

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

func joinAnd(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}
