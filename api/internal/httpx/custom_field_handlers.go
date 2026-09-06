package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"pms/internal/audit"
	"pms/internal/gen"
	"pms/internal/identity"
	"pms/internal/properties"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func customFieldToJSON(f properties.CustomField) map[string]any {
	choices := make([]map[string]any, 0, len(f.Choices))
	for _, c := range f.Choices {
		choices = append(choices, map[string]any{
			"id":    c.ID.String(),
			"label": c.Label,
		})
	}
	return map[string]any{
		"id":          f.ID.String(),
		"label":       f.Label,
		"fieldType":   string(f.FieldType),
		"isSensitive": f.IsSensitive,
		"choices":     choices,
		"valuesCount": f.ValuesCount,
	}
}

func customFieldListJSON(items []properties.CustomField) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, f := range items {
		out = append(out, customFieldToJSON(f))
	}
	return out
}

func (s *Server) handleListCustomFields() http.Handler {
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
		_ = c

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		items, err := s.properties.ListCustomFields(r.Context(), tx)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(customFieldListJSON(items))
	})
}

func (s *Server) handleCreateCustomField() http.Handler {
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
		if !requireAdmin(w, r, c) {
			return
		}
		var body gen.CustomFieldCreate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		label := strings.TrimSpace(identity.EasternToWesternDigits(body.Label))
		if n := len([]rune(label)); n < 1 || n > 60 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("label", MsgCustomFieldLabelLength))
			return
		}
		ft := properties.CustomFieldType(body.FieldType)
		switch ft {
		case properties.FieldText, properties.FieldAutocomplete, properties.FieldDropdown, properties.FieldMultiselect, properties.FieldCheckbox:
		default:
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("fieldType", MsgInvalidRequest))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		isSensitive := derefBool(body.IsSensitive)
		created, err := s.properties.CreateCustomField(r.Context(), tx,
			label, identity.Canonical(label), ft, isSensitive,
			trimAndValidateChoices(derefStrings(body.Choices)))
		if err != nil {
			switch err {
			case properties.ErrSensitiveTextOnly:
				// The store refuses sensitivity for every non-text type; the
				// Arabic message names the type the operator actually picked.
				sensitiveMsg := MsgCustomFieldSensitiveTextOnly
				if ft == properties.FieldAutocomplete {
					sensitiveMsg = MsgCustomFieldSensitiveAutocomplete
				}
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
					WithField("isSensitive", sensitiveMsg))
			case properties.ErrChoicesRequired:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
					WithField("choices", MsgCustomFieldChoicesRequired))
			case properties.ErrDuplicateLabel:
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgCustomFieldLabelTaken).
					WithField("label", MsgCustomFieldLabelTaken))
			default:
				refuseInternal(w, err)
			}
			return
		}
		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.CustomFieldCreated,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityCustomField,
			EntityID:       created.ID.String(),
			SourceIP:       ClientIP(r),
			After: map[string]any{
				"label":       label,
				"fieldType":   string(ft),
				"isSensitive": isSensitive,
			},
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(customFieldToJSON(*created))
	})
}

func (s *Server) handleUpdateCustomField() http.Handler {
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
		if !requireAdmin(w, r, c) {
			return
		}
		id, err := parseUUIDParam(r, "fieldId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}
		var body gen.CustomFieldUpdate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		label := strings.TrimSpace(identity.EasternToWesternDigits(body.Label))
		if n := len([]rune(label)); n < 1 || n > 60 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("label", MsgCustomFieldLabelLength))
			return
		}
		addChoices := trimAndValidateChoices(derefStrings(body.AddChoices))
		removeIDs := derefUUIDs(body.RemoveChoiceIds)

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		previousLabel, err := customFieldLabelByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if previousLabel == "" {
			_ = tx.Rollback(r.Context())
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgCustomFieldNotFound))
			return
		}

		updated, err := s.properties.UpdateCustomField(r.Context(), tx, id,
			label, identity.Canonical(label), addChoices, removeIDs)
		if err != nil {
			switch err {
			case properties.ErrInUseValuesExist:
				count, cntErr := s.properties.CountCustomFieldUsage(r.Context(), tx, id)
				if cntErr != nil {
					refuseInternal(w, cntErr)
					return
				}
				msg := MsgCustomFieldInUsePlural
				if count == 1 {
					msg = MsgCustomFieldInUseSingular
				}
				WriteError(w, NewAPIError(http.StatusConflict, CodeInUse, formatCountMsg(msg, count)))
				return
			case properties.ErrDuplicateLabel:
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgCustomFieldLabelTaken).
					WithField("label", MsgCustomFieldLabelTaken))
				return
			}
			refuseInternal(w, err)
			return
		}
		if updated == nil {
			_ = tx.Rollback(r.Context())
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgCustomFieldNotFound))
			return
		}
		actorID := c.Account.ID
		actorRole := c.Account.Role
		if previousLabel != label {
			if err := audit.Write(r.Context(), tx, audit.Entry{
				Action:         audit.CustomFieldRenamed,
				ActorAccountID: &actorID,
				ActorUsername:  c.Account.Username,
				ActorRole:      &actorRole,
				EntityType:     audit.EntityCustomField,
				EntityID:       updated.ID.String(),
				SourceIP:       ClientIP(r),
				Before:         map[string]any{"label": previousLabel},
				After:          map[string]any{"label": label},
			}); err != nil {
				refuseInternal(w, err)
				return
			}
		}
		for _, ch := range addChoices {
			if err := audit.Write(r.Context(), tx, audit.Entry{
				Action:         audit.CustomFieldChoiceAdded,
				ActorAccountID: &actorID,
				ActorUsername:  c.Account.Username,
				ActorRole:      &actorRole,
				EntityType:     audit.EntityCustomField,
				EntityID:       updated.ID.String(),
				SourceIP:       ClientIP(r),
				Detail:         map[string]any{"choice": ch},
			}); err != nil {
				refuseInternal(w, err)
				return
			}
		}
		for _, cid := range removeIDs {
			if err := audit.Write(r.Context(), tx, audit.Entry{
				Action:         audit.CustomFieldChoiceRemoved,
				ActorAccountID: &actorID,
				ActorUsername:  c.Account.Username,
				ActorRole:      &actorRole,
				EntityType:     audit.EntityCustomField,
				EntityID:       updated.ID.String(),
				SourceIP:       ClientIP(r),
				Detail:         map[string]any{"choiceId": cid.String()},
			}); err != nil {
				refuseInternal(w, err)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(customFieldToJSON(*updated))
	})
}

func (s *Server) handleDeleteCustomField() http.Handler {
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
		if !requireAdmin(w, r, c) {
			return
		}
		id, err := parseUUIDParam(r, "fieldId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		previousLabel, err := customFieldLabelByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if previousLabel == "" {
			_ = tx.Rollback(r.Context())
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgCustomFieldNotFound))
			return
		}

		if err := s.properties.DeleteCustomField(r.Context(), tx, id); err != nil {
			if err == properties.ErrInUseValuesExist {
				count, cntErr := s.properties.CountCustomFieldUsage(r.Context(), tx, id)
				if cntErr != nil {
					refuseInternal(w, cntErr)
					return
				}
				msg := MsgCustomFieldInUsePlural
				if count == 1 {
					msg = MsgCustomFieldInUseSingular
				}
				WriteError(w, NewAPIError(http.StatusConflict, CodeInUse, formatCountMsg(msg, count)))
				return
			}
			refuseInternal(w, err)
			return
		}
		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.CustomFieldRemoved,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityCustomField,
			EntityID:       id.String(),
			SourceIP:       ClientIP(r),
			Before:         map[string]any{"label": previousLabel},
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func trimAndValidateChoices(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, c := range raw {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if n := len([]rune(c)); n < 1 || n > 60 {
			continue
		}
		out = append(out, c)
	}
	return out
}

func derefStrings(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}

func derefUUIDs(values *[]uuid.UUID) []uuid.UUID {
	if values == nil {
		return nil
	}
	return *values
}

// customFieldLabelByID returns the current label for one custom field, or
// "" if no such row exists. The rename handler uses this to capture the
// before label so the audit row's before/after columns are complete.
func customFieldLabelByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (string, error) {
	var label string
	row := tx.QueryRow(ctx, `SELECT label FROM custom_field WHERE id = $1`, id)
	if err := row.Scan(&label); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return label, nil
}
