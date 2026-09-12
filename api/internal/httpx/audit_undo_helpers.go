// web/internal/httpx/audit_undo_helpers.go
// Helpers shared by the undo handler in audit_undo_handler.go. Kept in a
// separate file so the dispatch table stays readable.

package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"pms/internal/audit"
	"pms/internal/identity"
	"pms/internal/properties"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// parseUUIDPath parses a UUID string and returns a friendly undo error.
func parseUUIDPath(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, newUndoError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest)
	}
	return id, nil
}

func uuidParse(raw string) (uuid.UUID, error) {
	return parseUUIDPath(raw, "id")
}

// asString extracts a string from any, tolerating JSON's number-shaped
// uuids and other encoding artefacts. Returns "" for nil or wrong type.
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// canonicalNameForUndo applies the same normalisation the property handler
// uses for names so the restored row matches what the regular update path
// would have stored.
func canonicalNameForUndo(s string) string {
	return identity.Canonical(strings.TrimSpace(s))
}

// canonicalLabelForUndo applies the lookup normaliser.
func canonicalLabelForUndo(s string) string {
	return identity.Canonical(strings.TrimSpace(s))
}

// canonicalCodeForUndo applies the code normaliser.
func canonicalCodeForUndo(s string) string {
	return identity.Canonical(strings.TrimSpace(s))
}

// canonicalNoteForUndo trims the note text the way the archive handler does.
func canonicalNoteForUndo(s string) string {
	return strings.TrimSpace(s)
}

// customValueInputsFromAudit turns an audit row's `before.customValues`
// map back into the inputs the store's SaveCustomValues consumes. Sensitive
// fields must already be excluded from this map (the writer omits them);
// we still defend against a value of null by skipping it.
func customValueInputsFromAudit(ctx context.Context, cvRaw map[string]any, tx pgx.Tx) ([]properties.CustomValueInput, error) {
	out := make([]properties.CustomValueInput, 0, len(cvRaw))
	for fieldIDStr, value := range cvRaw {
		fid, err := uuid.Parse(strings.TrimSpace(fieldIDStr))
		if err != nil {
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
		}
		// Resolve the field definition once.
		var ft string
		var isSensitive bool
		row := tx.QueryRow(ctx,
			`SELECT field_type, is_sensitive FROM custom_field WHERE id = $1`, fid)
		if err := row.Scan(&ft, &isSensitive); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, newUndoError(http.StatusConflict, CodeConflict, MsgCustomFieldNotFound)
			}
			return nil, err
		}
		in := properties.CustomValueInput{
			FieldID:     fid,
			FieldType:   properties.CustomFieldType(ft),
			IsSensitive: isSensitive,
		}
		switch properties.CustomFieldType(ft) {
		case properties.FieldText:
			if value == nil {
				continue
			}
			if isSensitive {
				// // Should not happen: the writer omits sensitive values
				// from before.customValues. Refuse defensively.
				return nil, newUndoError(http.StatusConflict, CodeIntegrity, MsgAuditSensitiveUnrestorable)
			}
			s, ok := value.(string)
			if !ok {
				return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
			}
			in.Text = &s
		case properties.FieldCheckbox:
			if value == nil {
				continue
			}
			b, ok := value.(bool)
			if !ok {
				return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
			}
			in.Checked = &b
		case properties.FieldDropdown:
			if value == nil {
				continue
			}
			s, ok := value.(string)
			if !ok {
				return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
			}
			cid, err := uuid.Parse(s)
			if err != nil {
				return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
			}
			in.ChoiceID = &cid
		case properties.FieldMultiselect:
			if value == nil {
				continue
			}
			list, ok := value.([]any)
			if !ok {
				return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
			}
			for _, raw := range list {
				s, ok := raw.(string)
				if !ok {
					return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
				}
				cid, err := uuid.Parse(s)
				if err != nil {
					return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
				}
				in.ChoiceIDs = append(in.ChoiceIDs, cid)
			}
		default:
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
		}
		out = append(out, in)
	}
	return out, nil
}

func auditListNotEmpty(value any) bool {
	switch list := value.(type) {
	case []any:
		return len(list) > 0
	case []string:
		return len(list) > 0
	default:
		return false
	}
}

func auditDiffWithoutSensitive(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		if key != "sensitiveFields" {
			out[key] = value
		}
	}
	return out
}

// uuidFromAuditEntityID parses a property's UUID from the audit row's
// EntityID. For property writes the entity_id carries the property's
// reference code, NOT the UUID; for archive/restore we need the UUID. The
// archive handler stores the property_id column in `detail`, so we read
// it from there.
func uuidFromAuditEntityID(target *audit.Record) (uuid.UUID, error) {
	if target.EntityID != nil {
		if id, err := uuid.Parse(*target.EntityID); err == nil {
			return id, nil
		}
	}
	detail := target.DetailMap()
	if s, ok := detail["propertyId"].(string); ok {
		if id, err := uuid.Parse(s); err == nil {
			return id, nil
		}
	}
	return uuid.Nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
}

// resolvePropertyIDForUndo resolves the property's UUID for the
// property_restored case. property_restored's entity_id is the property's
// reference code (since it was written by the same audit path as
// property_archived); to get the UUID we look up the code.
func resolvePropertyIDForUndo(r *http.Request, s *Server, target *audit.Record) (uuid.UUID, error) {
	if target.EntityID != nil {
		if id, err := uuid.Parse(*target.EntityID); err == nil {
			return id, nil
		}
	}
	if target.EntityID == nil {
		return uuid.Nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}
	return resolvePropertyIDByCode(r, s, *target.EntityID)
}

// resolvePropertyIDByCode finds the property's UUID given its reference
// code. Used by property_code_changed (EntityID is the new code) and the
// restore path (EntityID is the reference code).
func resolvePropertyIDByCode(r *http.Request, s *Server, code string) (uuid.UUID, error) {
	if code == "" {
		return uuid.Nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}
	normalized := identity.Canonical(strings.TrimSpace(code))
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	row := tx.QueryRow(r.Context(), `SELECT id FROM property WHERE code_normalized = $1 LIMIT 1`, normalized)
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, newUndoError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound)
		}
		return uuid.Nil, err
	}
	return id, nil
}

// attachmentIDFromAudit pulls the attachment id out of an attachment audit
// row's detail. The attachment_* actions store the id in `detail` because
// entity_id carries the property, not the attachment.
func attachmentIDFromAudit(target *audit.Record) (uuid.UUID, error) {
	detail := target.DetailMap()
	if s, ok := detail["attachmentId"].(string); ok {
		id, err := uuid.Parse(s)
		if err != nil {
			return uuid.Nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
		}
		return id, nil
	}
	if target.EntityID != nil {
		if id, err := uuid.Parse(*target.EntityID); err == nil {
			return id, nil
		}
	}
	return uuid.Nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
}

// findChoiceIDByLabel resolves a custom-field choice id from its label.
// Refused if the choice was removed in the meantime.
func findChoiceIDByLabel(r *http.Request, s *Server, fieldID uuid.UUID, label string) (*uuid.UUID, error) {
	normalized := identity.Canonical(strings.TrimSpace(label))
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	row := tx.QueryRow(r.Context(),
		`SELECT id FROM custom_field_choice WHERE custom_field_id = $1 AND label_normalized = $2 LIMIT 1`,
		fieldID, normalized)
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &id, nil
}

// kindFromForAuditRecord maps an audit row's EntityType back to the
// properties.LookupKind used by the rename handler.
func kindFromForAuditRecord(t *audit.EntityType) properties.LookupKind {
	if t == nil {
		return ""
	}
	switch *t {
	case audit.EntityPropertyType:
		return properties.LookupPropertyType
	case audit.EntityArea:
		return properties.LookupArea
	}
	return ""
}
