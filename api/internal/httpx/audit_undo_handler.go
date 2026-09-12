// web/internal/httpx/audit_undo_handler.go
// Item 5 of the property-attachments-and-fixes brief. Undo reverses a recorded
// change by writing a new audited change through the normal store methods,
// so every constraint, version check, and audit write applies exactly as for
// a manual edit. The original record is never rewritten (Constitution VIII as
// amended in v1.4.0).

package httpx

import (
	"encoding/json"
	"errors"
	"net/http"

	"pms/internal/audit"
	"pms/internal/properties"

	"github.com/google/uuid"
)

// handleUndoAuditRecord reverses the change recorded by `recordId`. Refused
// with 409 when the underlying business record's version has moved on, or
// when the original change touched an encrypted field whose prior value
// cannot be restored.
func (s *Server) handleUndoAuditRecord(recordId int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := s.authenticate(w, r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if c == nil {
			WriteError(w, NewAPIError(http.StatusUnauthorized, CodeNotAuthenticated, MsgUnauthorized))
			return
		}

		// Load the recorded change outside a transaction so we can decide
		// whether anything needs to be done. GetByID never blocks the row;
		// it is read-only.
		target, err := audit.GetByID(r.Context(), s.pool, recordId)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if target == nil {
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgNotFound))
			return
		}

		// Once reversed, the change stays visible. The log gains a
		// reversal, never an erasure; undoing an already-undone change is
		// refused.
		count, err := audit.CountReversalsOf(r.Context(), s.pool, recordId)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if count > 0 {
			WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgAuditAlreadyUndone))
			return
		}

		// An undo of an undo is rejected too: the reversal pair is already
		// complete.
		if target.ReversesAuditID != nil {
			WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgAuditCannotUndoUndo))
			return
		}

		newRecord, err := s.performUndo(r, c, target)
		if err != nil {
			if apiErr, ok := err.(*undoError); ok {
				WriteError(w, apiErr.toAPIError())
				return
			}
			refuseInternal(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(newRecord)
	})
}

// undoError carries an HTTP status code and Arabic message from the
// per-action undo code back to the handler. The handler turns it into an
// API error body. This avoids threading error-handling tuples through
// every helper.
type undoError struct {
	status  int
	code    string
	message string
}

func (e *undoError) Error() string { return e.message }

func (e *undoError) toAPIError() *APIError {
	return NewAPIError(e.status, e.code, e.message)
}

func newUndoError(status int, code, message string) *undoError {
	return &undoError{status: status, code: code, message: message}
}

// performUndo dispatches on the recorded action and applies the inverse
// change through the standard store methods. Returns the new audit row, or
// an undoError describing why undo is refused.
func (s *Server) performUndo(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	switch target.Action {
	case audit.PropertyModified:
		return s.undoPropertyModified(r, c, target)
	case audit.PropertyArchived:
		return s.undoPropertyArchived(r, c, target)
	case audit.PropertyRestored:
		return s.undoPropertyRestored(r, c, target)
	case audit.PropertyCodeChanged:
		return s.undoPropertyCodeChanged(r, c, target)
	case audit.LookupRenamed:
		return s.undoLookupRenamed(r, c, target)
	case audit.CustomFieldRenamed:
		return s.undoCustomFieldRenamed(r, c, target)
	case audit.CustomFieldChoiceAdded:
		return s.undoCustomFieldChoiceAdded(r, c, target)
	case audit.CustomFieldChoiceRemoved:
		return s.undoCustomFieldChoiceRemoved(r, c, target)
	case audit.AttachmentDescribed:
		return s.undoAttachmentDescribed(r, c, target)
	default:
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}
}

// undoPropertyModified restores the property's name/type/area and any
// non-sensitive custom values that were recorded in `before`. Sensitive
// field changes cannot be undone (the row never recorded their prior
// value), so any prior change that touched a sensitive field refuses the
// whole undo.
func (s *Server) undoPropertyModified(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	beforeMap := target.BeforeMap()
	if beforeMap == nil {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}
	if auditListNotEmpty(beforeMap["sensitiveFields"]) {
		return nil, newUndoError(http.StatusConflict, CodeIntegrity, MsgAuditSensitiveUnrestorable)
	}
	propertyID, err := resolvePropertyIDForUndo(r, s, target)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	current, err := s.properties.FindByID(r.Context(), tx, propertyID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, newUndoError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound)
	}

	name := current.Name
	if beforeName, ok := beforeMap["name"]; ok {
		name = asString(beforeName)
	}
	propertyTypeID := current.PropertyTypeID
	if beforeTypeID, ok := beforeMap["propertyTypeId"]; ok {
		propertyTypeID, err = uuidParse(asString(beforeTypeID))
		if err != nil {
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
		}
	}
	areaID := current.AreaID
	if beforeAreaID, ok := beforeMap["areaId"]; ok {
		areaID, err = uuidParse(asString(beforeAreaID))
		if err != nil {
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
		}
	}

	// The store's UpdateProperty refuses on version conflict, archived, and
	// unknown type/area — exactly the safety net the brief requires.
	updated, err := s.properties.UpdateProperty(r.Context(), tx, propertyID,
		name, canonicalNameForUndo(name),
		propertyTypeID, areaID, c.Account.ID, current.Version)
	if err != nil {
		switch {
		case errors.Is(err, properties.ErrArchived):
			return nil, newUndoError(http.StatusConflict, CodeArchived, MsgArchived)
		case errors.Is(err, properties.ErrVersionConflict):
			return nil, newUndoError(http.StatusConflict, CodeVersionConflict, MsgAuditVersionMoved)
		}
		return nil, err
	}

	if cvRaw, ok := beforeMap["customValues"].(map[string]any); ok {
		fieldIDs := make([]uuid.UUID, 0, len(cvRaw))
		for fieldID := range cvRaw {
			id, err := uuid.Parse(fieldID)
			if err != nil {
				return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
			}
			fieldIDs = append(fieldIDs, id)
		}
		if err := s.properties.DeleteCustomValuesForFields(r.Context(), tx, propertyID, fieldIDs); err != nil {
			return nil, err
		}
		inputs, apiErr := customValueInputsFromAudit(r.Context(), cvRaw, tx)
		if apiErr != nil {
			return nil, apiErr
		}
		if len(inputs) > 0 {
			if err := s.properties.SaveCustomValues(r.Context(), tx, s.envelope, propertyID, inputs); err != nil {
				return nil, err
			}
		}
	}

	actorID := c.Account.ID
	actorRole := c.Account.Role
	undoBefore := auditDiffWithoutSensitive(target.AfterMap())
	undoAfter := auditDiffWithoutSensitive(beforeMap)
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      audit.EntityProperty,
		EntityID:        updated.Code,
		SourceIP:        ClientIP(r),
		Before:          undoBefore,
		After:           undoAfter,
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// undoPropertyArchived reverses an archive by restoring the property.
func (s *Server) undoPropertyArchived(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	propertyID, err := resolvePropertyIDForUndo(r, s, target)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	current, err := s.properties.FindByID(r.Context(), tx, propertyID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, newUndoError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound)
	}
	if current.ArchivedAt == nil {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditAlreadyActive)
	}

	restored, err := s.properties.Restore(r.Context(), tx, propertyID, c.Account.ID, current.Version)
	if err != nil {
		switch {
		case errors.Is(err, properties.ErrArchived):
			return nil, newUndoError(http.StatusConflict, CodeArchived, MsgArchived)
		case errors.Is(err, properties.ErrVersionConflict):
			return nil, newUndoError(http.StatusConflict, CodeVersionConflict, MsgAuditVersionMoved)
		}
		return nil, err
	}
	actorID := c.Account.ID
	actorRole := c.Account.Role
	undoBefore := map[string]any{"isArchived": true, "archiveNote": nil}
	if current.ArchiveNote != nil {
		undoBefore["archiveNote"] = *current.ArchiveNote
	}
	undoAfter := map[string]any{"isArchived": false, "archiveNote": nil}
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      audit.EntityProperty,
		EntityID:        restored.Code,
		SourceIP:        ClientIP(r),
		Before:          undoBefore,
		After:           undoAfter,
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// undoPropertyRestored reverses a restore by archiving again with the same
// note.
func (s *Server) undoPropertyRestored(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	propertyID, err := resolvePropertyIDForUndo(r, s, target)
	if err != nil {
		if ue, ok := err.(*undoError); ok {
			return nil, ue
		}
		return nil, err
	}

	beforeMap := target.BeforeMap()
	note := asString(beforeMap["archiveNote"])

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	current, err := s.properties.FindByID(r.Context(), tx, propertyID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, newUndoError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound)
	}
	if current.ArchivedAt != nil {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditAlreadyArchived)
	}

	var notePtr *string
	if trimmed := canonicalNoteForUndo(note); trimmed != "" {
		notePtr = &trimmed
	}
	archived, err := s.properties.Archive(r.Context(), tx, propertyID, c.Account.ID, current.Version, notePtr)
	if err != nil {
		switch {
		case errors.Is(err, properties.ErrArchived):
			return nil, newUndoError(http.StatusConflict, CodeArchived, MsgArchived)
		case errors.Is(err, properties.ErrVersionConflict):
			return nil, newUndoError(http.StatusConflict, CodeVersionConflict, MsgAuditVersionMoved)
		}
		return nil, err
	}

	actorID := c.Account.ID
	actorRole := c.Account.Role
	undoBefore := map[string]any{"isArchived": false, "archiveNote": nil}
	undoAfter := map[string]any{"isArchived": true, "archiveNote": nil}
	if archived.ArchiveNote != nil {
		undoAfter["archiveNote"] = *archived.ArchiveNote
	}
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      audit.EntityProperty,
		EntityID:        archived.Code,
		SourceIP:        ClientIP(r),
		Before:          undoBefore,
		After:           undoAfter,
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// undoPropertyCodeChanged reverses a code change by setting the code back
// to its prior value.
func (s *Server) undoPropertyCodeChanged(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	beforeMap := target.BeforeMap()
	oldCode := asString(beforeMap["code"])
	if oldCode == "" {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}

	if target.EntityID == nil || *target.EntityID == "" {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}
	propertyID, err := resolvePropertyIDByCode(r, s, *target.EntityID)
	if err != nil {
		if ue, ok := err.(*undoError); ok {
			return nil, ue
		}
		return nil, err
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	current, err := s.properties.FindByID(r.Context(), tx, propertyID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, newUndoError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound)
	}

	oldNormalized := canonicalCodeForUndo(oldCode)
	_, _, err = s.properties.ChangeCode(r.Context(), tx, propertyID, oldCode, oldNormalized, c.Account.ID, current.Version)
	if err != nil {
		switch {
		case errors.Is(err, properties.ErrArchived):
			return nil, newUndoError(http.StatusConflict, CodeArchived, MsgArchived)
		case errors.Is(err, properties.ErrVersionConflict):
			return nil, newUndoError(http.StatusConflict, CodeVersionConflict, MsgAuditVersionMoved)
		case errors.Is(err, properties.ErrDuplicateCode):
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditRestoreCollision)
		}
		return nil, err
	}

	actorID := c.Account.ID
	actorRole := c.Account.Role
	undoBefore := map[string]any{"code": current.Code}
	undoAfter := map[string]any{"code": oldCode}
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      audit.EntityProperty,
		EntityID:        oldCode,
		SourceIP:        ClientIP(r),
		Before:          undoBefore,
		After:           undoAfter,
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// undoLookupRenamed reverses a property-type or area rename.
func (s *Server) undoLookupRenamed(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	kind := kindFromForAuditRecord(target.EntityType)
	if kind == "" {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}
	if target.EntityID == nil {
		return nil, newUndoError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest)
	}
	id, err := uuidParse(*target.EntityID)
	if err != nil {
		return nil, err
	}
	beforeMap := target.BeforeMap()
	oldLabel := asString(beforeMap["label"])
	if oldLabel == "" {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	updated, err := s.properties.RenameLookup(r.Context(), tx, kind, id, oldLabel, canonicalLabelForUndo(oldLabel))
	if err != nil {
		if errors.Is(err, properties.ErrLookupDuplicateLabel) {
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditRestoreCollision)
		}
		return nil, err
	}
	if updated == nil {
		return nil, newUndoError(http.StatusNotFound, CodeNotFound, MsgLookupNotFound)
	}

	actorID := c.Account.ID
	actorRole := c.Account.Role
	undoBefore := map[string]any{"label": updated.Label}
	undoAfter := map[string]any{"label": oldLabel}
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      target.EntityTypeEnum(),
		EntityID:        id.String(),
		SourceIP:        ClientIP(r),
		Before:          undoBefore,
		After:           undoAfter,
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// undoCustomFieldRenamed reverses a custom-field rename by setting the
// label back to its prior value.
func (s *Server) undoCustomFieldRenamed(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	if target.EntityID == nil {
		return nil, newUndoError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest)
	}
	id, err := uuidParse(*target.EntityID)
	if err != nil {
		return nil, err
	}
	beforeMap := target.BeforeMap()
	oldLabel := asString(beforeMap["label"])
	if oldLabel == "" {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	updated, err := s.properties.UpdateCustomField(r.Context(), tx, id, oldLabel, canonicalLabelForUndo(oldLabel), nil, nil)
	if err != nil {
		switch {
		case errors.Is(err, properties.ErrDuplicateLabel):
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditRestoreCollision)
		case errors.Is(err, properties.ErrInUseValuesExist):
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
		}
		return nil, err
	}
	if updated == nil {
		return nil, newUndoError(http.StatusNotFound, CodeNotFound, MsgCustomFieldNotFound)
	}

	actorID := c.Account.ID
	actorRole := c.Account.Role
	undoBefore := map[string]any{"label": updated.Label}
	undoAfter := map[string]any{"label": oldLabel}
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      audit.EntityCustomField,
		EntityID:        id.String(),
		SourceIP:        ClientIP(r),
		Before:          undoBefore,
		After:           undoAfter,
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// undoCustomFieldChoiceAdded reverses by removing the choice that was
// added. Refused if any property holds the choice.
func (s *Server) undoCustomFieldChoiceAdded(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	if target.EntityID == nil {
		return nil, newUndoError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest)
	}
	id, err := uuidParse(*target.EntityID)
	if err != nil {
		return nil, err
	}
	detail := target.DetailMap()
	choiceLabel := asString(detail["choice"])
	if choiceLabel == "" {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}
	choiceID, err := findChoiceIDByLabel(r, s, id, choiceLabel)
	if err != nil {
		return nil, err
	}
	if choiceID == nil {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	if _, err := s.properties.UpdateCustomField(r.Context(), tx, id, "", "", nil, []uuid.UUID{*choiceID}); err != nil {
		if errors.Is(err, properties.ErrInUseValuesExist) {
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
		}
		return nil, err
	}

	actorID := c.Account.ID
	actorRole := c.Account.Role
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      audit.EntityCustomField,
		EntityID:        id.String(),
		SourceIP:        ClientIP(r),
		Before:          map[string]any{"choice": choiceLabel},
		After:           map[string]any{"choice": nil},
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// undoCustomFieldChoiceRemoved reverses by re-adding the choice.
func (s *Server) undoCustomFieldChoiceRemoved(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	if target.EntityID == nil {
		return nil, newUndoError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest)
	}
	id, err := uuidParse(*target.EntityID)
	if err != nil {
		return nil, err
	}
	beforeMap := target.BeforeMap()
	choiceLabel := asString(beforeMap["choice"])
	if choiceLabel == "" {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	if _, err := s.properties.UpdateCustomField(r.Context(), tx, id, "", "", []string{choiceLabel}, nil); err != nil {
		if errors.Is(err, properties.ErrDuplicateLabel) {
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditRestoreCollision)
		}
		return nil, err
	}

	actorID := c.Account.ID
	actorRole := c.Account.Role
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      audit.EntityCustomField,
		EntityID:        id.String(),
		SourceIP:        ClientIP(r),
		Before:          map[string]any{"choice": nil},
		After:           map[string]any{"choice": choiceLabel},
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// undoAttachmentDescribed reverses a description change. Refused if the
// attachment has since been removed or if its property has been archived.
func (s *Server) undoAttachmentDescribed(r *http.Request, c *CurrentAccount, target *audit.Record) (map[string]any, error) {
	beforeMap := target.BeforeMap()
	oldDescription := asString(beforeMap["description"])
	if oldDescription == "" {
		return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
	}
	attachmentID, err := attachmentIDFromAudit(target)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	a, err := loadAttachmentRow(r.Context(), tx, attachmentID)
	if err != nil {
		if errors.Is(err, properties.ErrNotFound) {
			return nil, newUndoError(http.StatusConflict, CodeConflict, MsgAuditUnrestorable)
		}
		return nil, err
	}
	if archived, _ := isPropertyArchived(r.Context(), tx, a.PropertyID); archived {
		return nil, newUndoError(http.StatusConflict, CodeArchived, MsgAttachmentArchivedRefused)
	}

	if _, err := tx.Exec(r.Context(), `
		UPDATE attachment SET description = $2, updated_at = now(), updated_by = $3
		WHERE id = $1
	`, attachmentID, oldDescription, c.Account.ID); err != nil {
		return nil, err
	}

	actorID := c.Account.ID
	actorRole := c.Account.Role
	undoBefore := map[string]any{"description": a.Description}
	undoAfter := map[string]any{"description": oldDescription}
	if err := audit.Write(r.Context(), tx, audit.Entry{
		Action:          audit.RecordReverted,
		ActorAccountID:  &actorID,
		ActorUsername:   c.Account.Username,
		ActorRole:       &actorRole,
		EntityType:      audit.EntityProperty,
		EntityID:        a.PropertyID.String(),
		SourceIP:        ClientIP(r),
		Before:          undoBefore,
		After:           undoAfter,
		ReversesAuditID: &target.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return fetchFreshUndoRecord(r, s, target.ID)
}

// helper — fresh audit record JSON for the new `record_reverted` row.
func fetchFreshUndoRecord(r *http.Request, s *Server, reversesID int64) (map[string]any, error) {
	rev, err := audit.GetByReversesID(r.Context(), s.pool, reversesID)
	if err != nil {
		return nil, err
	}
	if rev == nil {
		return nil, errors.New("undo audit row not found after commit")
	}
	return auditRecordToJSON(rev), nil
}

// auditRecordToJSON shapes an audit record into the same JSON form the
// list endpoint returns, including the freshly-set `reversesAuditId` and
// the resolved `revertedByAuditId`.
func auditRecordToJSON(r *audit.Record) map[string]any {
	rec := map[string]any{
		"id":            r.ID,
		"occurredAt":    r.OccurredAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"action":        string(r.Action),
		"actorUsername": r.ActorUsername,
		"sourceIp":      r.SourceIP.String(),
	}
	if r.ActorID != nil {
		rec["actorId"] = r.ActorID.String()
	} else {
		rec["actorId"] = nil
	}
	if r.ActorRole != nil {
		rec["actorRole"] = *r.ActorRole
	} else {
		rec["actorRole"] = nil
	}
	if r.EntityType != nil {
		rec["entityType"] = string(*r.EntityType)
	} else {
		rec["entityType"] = nil
	}
	if r.EntityID != nil {
		rec["entityId"] = *r.EntityID
	} else {
		rec["entityId"] = nil
	}
	if r.TargetID != nil {
		rec["targetId"] = r.TargetID.String()
	} else {
		rec["targetId"] = nil
	}
	if r.TargetUsername != nil {
		rec["targetUsername"] = *r.TargetUsername
	} else {
		rec["targetUsername"] = nil
	}
	if len(r.Detail) > 0 {
		rec["detail"] = json.RawMessage(r.Detail)
	} else {
		rec["detail"] = nil
	}
	if len(r.Before) > 0 {
		rec["before"] = json.RawMessage(r.Before)
	} else {
		rec["before"] = nil
	}
	if len(r.After) > 0 {
		rec["after"] = json.RawMessage(r.After)
	} else {
		rec["after"] = nil
	}
	if r.ReversesAuditID != nil {
		rec["reversesAuditId"] = *r.ReversesAuditID
	} else {
		rec["reversesAuditId"] = nil
	}
	return rec
}
