// Package audit writes the append-only audit_log rows and serves the records
// screen. Every write happens inside the caller's transaction, so the audit
// row and the change it records commit or roll back together (Constitution
// VIII).
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	// Feature 003-property-attachments-ocr. Attachment audit rows reuse
	// EntityProperty because the question these records answer is "what
	// happened to this property", and an attachment that has been removed
	// still has to appear under the property it belonged to (data-model.md
	// "Entity relationships"). Attachment identifier and other identifying
	// detail go in the detail JSON, not in entity_id.
	AttachmentAdded           Action = "attachment_added"
	AttachmentDescribed       Action = "attachment_described"
	AttachmentSensitivityRaised Action = "attachment_sensitivity_raised"
	AttachmentRemoved         Action = "attachment_removed"
	AttachmentRead            Action = "attachment_read"
	AttachmentTextCorrected   Action = "attachment_text_corrected"
	AttachmentReextractRequested Action = "attachment_reextract_requested"

	// Feature: undo system (Constitution VIII as amended in v1.4.0). An undo
	// is itself an audited change that points back at the record it reverses;
	// no application code path may update or delete an audit row.
	RecordReverted Action = "record_reverted"
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
	Before          any // JSON-serialisable; values stored unencrypted only
	After           any // JSON-serialisable; values stored unencrypted only
	ReversesAuditID *int64
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
	var beforeJSON []byte
	if e.Before != nil {
		b, err := json.Marshal(e.Before)
		if err != nil {
			return err
		}
		beforeJSON = b
	}
	var afterJSON []byte
	if e.After != nil {
		b, err := json.Marshal(e.After)
		if err != nil {
			return err
		}
		afterJSON = b
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
			source_ip, detail,
			before_value, after_value, reverses_audit_id
		) VALUES (
			now(), $1,
			$2, $3, $4,
			$5, $6,
			$7, $8,
			$9, $10,
			$11, $12, $13
		)`,
		string(e.Action),
		e.ActorAccountID, e.ActorUsername, actorRole,
		entityType, entityID,
		e.TargetAccountID, e.TargetUsername,
		e.SourceIP, detailJSON,
		beforeJSON, afterJSON, e.ReversesAuditID,
	)
	return err
}

// WriteCommitted writes an audit row on its own transaction and commits it
// before returning. Used for attachment reads (research.md D-010): a read
// cannot be rolled back once bytes are on the wire, so the row has to be
// durable first. If the audit write fails the caller must NOT serve any
// content; the row never existing is the safe direction.
//
// Over-recording a read that fails mid-stream is the deliberate trade
// (research.md D-010): an "attachment_read" row for a download that aborted
// half-way is honest about what happened.
func WriteCommitted(ctx context.Context, pool interface {
	Begin(context.Context) (pgx.Tx, error)
}, e Entry) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("audit begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := Write(ctx, tx, e); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("audit commit: %w", err)
	}
	return nil
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
	Before          []byte
	After           []byte
	ReversesAuditID *int64
}

// EntityTypeEnum returns the EntityType as a value (never a pointer). The
// undo handler needs to pass a non-nil EntityType to audit.Write.
func (r *Record) EntityTypeEnum() EntityType {
	if r.EntityType == nil {
		return ""
	}
	return *r.EntityType
}

// BeforeMap returns the Before column as a decoded map, or nil if it is
// absent. The undo flow uses this in many places; a single helper keeps
// the nil-vs-empty handling consistent.
func (r *Record) BeforeMap() map[string]any {
	if len(r.Before) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(r.Before, &m); err != nil {
		return nil
	}
	return m
}

// AfterMap returns the After column as a decoded map, or nil.
func (r *Record) AfterMap() map[string]any {
	if len(r.After) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(r.After, &m); err != nil {
		return nil
	}
	return m
}

// DetailMap returns the Detail column as a decoded map, or nil.
func (r *Record) DetailMap() map[string]any {
	if len(r.Detail) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(r.Detail, &m); err != nil {
		return nil
	}
	return m
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
		       source_ip, detail,
		       before_value, after_value, reverses_audit_id
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
		var detail, before, after []byte
		if err := rows.Scan(&r.ID, &r.OccurredAt, &r.Action,
			&r.ActorID, &r.ActorUsername, &r.ActorRole,
			&r.EntityType, &r.EntityID,
			&r.TargetID, &r.TargetUsername,
			&r.SourceIP, &detail,
			&before, &after, &r.ReversesAuditID); err != nil {
			return nil, 0, err
		}
		r.Detail = detail
		r.Before = before
		r.After = after
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetByID returns a single audit record by id, or nil if no such record
// exists. Used by the undo endpoint to load the change being reversed.
func GetByID(ctx context.Context, pool *pgxpool.Pool, id int64) (*Record, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, occurred_at, action,
		       actor_account_id, actor_username_snapshot, actor_role,
		       entity_type, entity_id,
		       target_account_id, target_username_snapshot,
		       source_ip, detail,
		       before_value, after_value, reverses_audit_id
		FROM audit_log
		WHERE id = $1`, id)
	r := &Record{}
	var detail, before, after []byte
	if err := row.Scan(&r.ID, &r.OccurredAt, &r.Action,
		&r.ActorID, &r.ActorUsername, &r.ActorRole,
		&r.EntityType, &r.EntityID,
		&r.TargetID, &r.TargetUsername,
		&r.SourceIP, &detail,
		&before, &after, &r.ReversesAuditID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	r.Detail = detail
	r.Before = before
	r.After = after
	return r, nil
}

// CountReversalsOf returns the number of `record_reverted` rows whose
// `reverses_audit_id` points at the given id. Used to refuse undoing a
// record that has already been undone.
func CountReversalsOf(ctx context.Context, pool *pgxpool.Pool, id int64) (int, error) {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		WHERE reverses_audit_id = $1 AND action = 'record_reverted'`, id).Scan(&n)
	return n, err
}

// GetByReversesID returns the most recent audit record that reverses the
// given id (the new `record_reverted` row), or nil if none exists. Used by
// the undo handler to surface the new row it just wrote.
func GetByReversesID(ctx context.Context, pool *pgxpool.Pool, id int64) (*Record, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, occurred_at, action,
		       actor_account_id, actor_username_snapshot, actor_role,
		       entity_type, entity_id,
		       target_account_id, target_username_snapshot,
		       source_ip, detail,
		       before_value, after_value, reverses_audit_id
		FROM audit_log
		WHERE reverses_audit_id = $1
		ORDER BY id DESC
		LIMIT 1`, id)
	r := &Record{}
	var detail, before, after []byte
	if err := row.Scan(&r.ID, &r.OccurredAt, &r.Action,
		&r.ActorID, &r.ActorUsername, &r.ActorRole,
		&r.EntityType, &r.EntityID,
		&r.TargetID, &r.TargetUsername,
		&r.SourceIP, &detail,
		&before, &after, &r.ReversesAuditID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	r.Detail = detail
	r.Before = before
	r.After = after
	return r, nil
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
