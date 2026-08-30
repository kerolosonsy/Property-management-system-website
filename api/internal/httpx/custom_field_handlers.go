package httpx

import (
	"encoding/json"
	"net/http"
	"strings"

	"pms/internal/audit"
	"pms/internal/identity"
	"pms/internal/properties"

	"github.com/google/uuid"
)

type customFieldCreateBody struct {
	Label       string   `json:"label"`
	FieldType   string   `json:"fieldType"`
	IsSensitive bool     `json:"isSensitive"`
	Choices     []string `json:"choices"`
}

type customFieldUpdateBody struct {
	Label          string   `json:"label"`
	AddChoices     []string `json:"addChoices"`
	RemoveChoiceIDs []string `json:"removeChoiceIds"`
}

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
		var body customFieldCreateBody
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
		case properties.FieldText, properties.FieldDropdown, properties.FieldMultiselect, properties.FieldCheckbox:
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

		created, err := s.properties.CreateCustomField(r.Context(), tx,
			label, identity.Canonical(label), ft, body.IsSensitive,
			trimAndValidateChoices(body.Choices))
		if err != nil {
			switch err {
			case properties.ErrSensitiveTextOnly:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
					WithField("isSensitive", MsgCustomFieldSensitiveTextOnly))
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
			Detail: map[string]any{
				"label":       label,
				"fieldType":   string(ft),
				"isSensitive": body.IsSensitive,
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
		var body customFieldUpdateBody
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
		addChoices := trimAndValidateChoices(body.AddChoices)
		removeIDs := make([]uuid.UUID, 0, len(body.RemoveChoiceIDs))
		for _, raw := range body.RemoveChoiceIDs {
			cid, err := uuid.Parse(raw)
			if err != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
					WithField("removeChoiceIds", MsgInvalidRequest))
				return
			}
			removeIDs = append(removeIDs, cid)
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

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
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.CustomFieldRenamed,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityCustomField,
			EntityID:       updated.ID.String(),
			SourceIP:       ClientIP(r),
			Detail: map[string]any{
				"addedChoices":   len(addChoices),
				"removedChoices": len(removeIDs),
			},
		}); err != nil {
			refuseInternal(w, err)
			return
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
