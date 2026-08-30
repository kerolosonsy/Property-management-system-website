package httpx

import (
	"encoding/json"
	"net/http"
	"strings"

	"pms/internal/audit"
	"pms/internal/identity"
	"pms/internal/properties"
)

type lookupWriteBody struct {
	Label string `json:"label"`
}

func lookupToJSON(l properties.Lookup) map[string]any {
	return map[string]any{
		"id":    l.ID.String(),
		"label": l.Label,
	}
}

func lookupListJSON(items []properties.Lookup) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, l := range items {
		out = append(out, lookupToJSON(l))
	}
	return out
}

func requireAdmin(w http.ResponseWriter, r *http.Request, c *CurrentAccount) bool {
	if c.Account.Role != "admin" {
		WriteError(w, NewAPIError(http.StatusForbidden, CodeForbidden, MsgForbidden))
		return false
	}
	return true
}

func (s *Server) handleListPropertyTypes() http.Handler {
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

		items, err := s.properties.ListLookups(r.Context(), tx, properties.LookupPropertyType)
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
		_ = json.NewEncoder(w).Encode(lookupListJSON(items))
	})
}

func (s *Server) handleListAreas() http.Handler {
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

		items, err := s.properties.ListLookups(r.Context(), tx, properties.LookupArea)
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
		_ = json.NewEncoder(w).Encode(lookupListJSON(items))
	})
}

func (s *Server) handleCreatePropertyType() http.Handler {
	return s.handleCreateLookup(properties.LookupPropertyType, audit.LookupCreated, "property_type")
}

func (s *Server) handleCreateArea() http.Handler {
	return s.handleCreateLookup(properties.LookupArea, audit.LookupCreated, "area")
}

func (s *Server) handleCreateLookup(kind properties.LookupKind, action audit.Action, entityName string) http.Handler {
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
		var body lookupWriteBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		label := strings.TrimSpace(identity.EasternToWesternDigits(body.Label))
		if n := len([]rune(label)); n < 1 || n > 60 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("label", MsgLookupLabelLength))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		created, err := s.properties.CreateLookup(r.Context(), tx, kind, label, identity.Canonical(label))
		if err != nil {
			if err == properties.ErrLookupDuplicateLabel {
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgLookupLabelTaken).
					WithField("label", MsgLookupLabelTaken))
				return
			}
			refuseInternal(w, err)
			return
		}
		actorID := c.Account.ID
		actorRole := c.Account.Role
		entityType := audit.EntityPropertyType
		if kind == properties.LookupArea {
			entityType = audit.EntityArea
		}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         action,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     entityType,
			EntityID:       created.ID.String(),
			SourceIP:       ClientIP(r),
			Detail:         map[string]any{"label": label, "kind": entityName},
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
		_ = json.NewEncoder(w).Encode(lookupToJSON(*created))
	})
}

func (s *Server) handleRenamePropertyType() http.Handler {
	return s.handleRenameLookup(properties.LookupPropertyType, audit.LookupRenamed)
}

func (s *Server) handleRenameArea() http.Handler {
	return s.handleRenameLookup(properties.LookupArea, audit.LookupRenamed)
}

func (s *Server) handleRenameLookup(kind properties.LookupKind, action audit.Action) http.Handler {
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
		id, err := parseUUIDParam(r, "lookupId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}
		var body lookupWriteBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		label := strings.TrimSpace(identity.EasternToWesternDigits(body.Label))
		if n := len([]rune(label)); n < 1 || n > 60 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("label", MsgLookupLabelLength))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		updated, err := s.properties.RenameLookup(r.Context(), tx, kind, id, label, identity.Canonical(label))
		if err != nil {
			if err == properties.ErrLookupDuplicateLabel {
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgLookupLabelTaken).
					WithField("label", MsgLookupLabelTaken))
				return
			}
			refuseInternal(w, err)
			return
		}
		if updated == nil {
			_ = tx.Rollback(r.Context())
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgLookupNotFound))
			return
		}
		actorID := c.Account.ID
		actorRole := c.Account.Role
		entityType := audit.EntityPropertyType
		if kind == properties.LookupArea {
			entityType = audit.EntityArea
		}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         action,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     entityType,
			EntityID:       updated.ID.String(),
			SourceIP:       ClientIP(r),
			Detail:         map[string]any{"label": label},
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(lookupToJSON(*updated))
	})
}

func (s *Server) handleDeletePropertyType() http.Handler {
	return s.handleDeleteLookup(properties.LookupPropertyType, audit.LookupRemoved)
}

func (s *Server) handleDeleteArea() http.Handler {
	return s.handleDeleteLookup(properties.LookupArea, audit.LookupRemoved)
}

func (s *Server) handleDeleteLookup(kind properties.LookupKind, action audit.Action) http.Handler {
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
		id, err := parseUUIDParam(r, "lookupId")
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

		count, err := s.properties.CountLookupUsage(r.Context(), tx, kind, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if count > 0 {
			_ = tx.Rollback(r.Context())
			msg := MsgLookupInUsePlural
			if count == 1 {
				msg = MsgLookupInUseSingular
			}
			WriteError(w, NewAPIError(http.StatusConflict, CodeInUse, formatCountMsg(msg, count)))
			return
		}
		if err := s.properties.DeleteLookup(r.Context(), tx, kind, id); err != nil {
			if err == properties.ErrInUse {
				WriteError(w, NewAPIError(http.StatusConflict, CodeInUse, MsgInUse))
				return
			}
			refuseInternal(w, err)
			return
		}
		actorID := c.Account.ID
		actorRole := c.Account.Role
		entityType := audit.EntityPropertyType
		if kind == properties.LookupArea {
			entityType = audit.EntityArea
		}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         action,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     entityType,
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

func formatCountMsg(template string, n int) string {
	out := template
	// Two forms: "{count}" only.
	for {
		start := -1
		for i := 0; i < len(out)-1; i++ {
			if out[i] == '{' {
				start = i
				continue
			}
			if out[i] == '}' && start >= 0 {
				out = out[:start] + itoa(n) + out[i+1:]
				start = -1
				break
			}
		}
		if start == -1 {
			break
		}
	}
	return out
}
