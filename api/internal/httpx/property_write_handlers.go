package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"pms/internal/audit"
	"pms/internal/gen"
	"pms/internal/identity"
	"pms/internal/properties"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// normalisePropertyName applies the trim + collapse rule (FR-016) before
// storage and normalisation.
func normalisePropertyName(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace && b.Len() > 0 {
				b.WriteRune(' ')
				inSpace = true
			}
			continue
		}
		b.WriteRune(r)
		inSpace = false
	}
	return strings.TrimRight(b.String(), " ")
}

func (s *Server) handleCreateProperty() http.Handler {
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

		var body gen.PropertyCreate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		body.Name = identity.EasternToWesternDigits(body.Name)

		name := normalisePropertyName(body.Name)
		if n := len([]rune(name)); n < 2 || n > 120 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("name", MsgPropertyNameLength))
			return
		}

		// A manager supplying code is refused with 403, per FR-017c. The
		// flat role check lives here so a single body field does not change
		// authorization (Constitution I).
		if body.Code != nil && *body.Code != "" && c.Account.Role != "admin" {
			WriteError(w, NewAPIError(http.StatusForbidden, CodeForbidden, MsgForbidden))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		var code, codeNorm string
		if body.Code != nil && *body.Code != "" {
			raw := strings.TrimSpace(*body.Code)
			if n := len([]rune(raw)); n < 1 || n > 32 {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
					WithField("code", MsgPropertyCodeLength))
				return
			}
			codeNorm = identity.Canonical(raw)
			free, archived, err := s.properties.VerifyCodeIsAvailable(r.Context(), tx, codeNorm)
			if err != nil {
				refuseInternal(w, err)
				return
			}
			if !free {
				if archived {
					WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgPropertyCodeArchived).
						WithField("code", MsgPropertyCodeArchived))
					return
				}
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgPropertyCodeTaken).
					WithField("code", MsgPropertyCodeTaken))
				return
			}
			code = raw
		} else {
			gen, err := s.properties.GenerateCode(r.Context(), tx)
			if err != nil {
				refuseInternal(w, err)
				return
			}
			code = gen
			codeNorm = identity.Canonical(gen)
		}

		// Validate the chosen type and area exist; a free-typed value
		// becomes a foreign-key violation otherwise.
		if !s.lookupExists(r.Context(), tx, "property_type", body.PropertyTypeId) {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("propertyTypeId", MsgPropertyTypeRequired))
			return
		}
		if !s.lookupExists(r.Context(), tx, "area", body.AreaId) {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("areaId", MsgPropertyAreaRequired))
			return
		}

		nameNorm := identity.Canonical(name)
		created, err := s.properties.CreateProperty(r.Context(), tx,
			name, nameNorm, code, codeNorm,
			body.PropertyTypeId, body.AreaId, c.Account.ID)
		if err != nil {
			if err == properties.ErrDuplicateCode {
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgPropertyCodeTaken).
					WithField("code", MsgPropertyCodeTaken))
				return
			}
			refuseInternal(w, err)
			return
		}

		customValues := customValueBodies(body.CustomValues)
		if len(customValues) > 0 {
			defs, err := s.loadCustomValueDefinitions(r.Context(), tx, customValues)
			if err != nil {
				refuseInternal(w, err)
				return
			}
			inputs, apiErr := buildCustomValueInputs(customValues, defs)
			if apiErr != nil {
				WriteError(w, apiErr)
				return
			}
			if err := s.properties.SaveCustomValues(r.Context(), tx, s.envelope, created.ID, inputs); err != nil {
				refuseInternal(w, err)
				return
			}
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.PropertyCreated,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       code,
			SourceIP:       ClientIP(r),
			After:          map[string]any{"name": name, "code": code},
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		full, err := s.fetchFullProperty(r.Context(), created.ID)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(full)
	})
}

// handleUpdateProperty implements PUT /properties/{propertyId}.
func (s *Server) handleUpdateProperty() http.Handler {
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
		id, err := parseUUIDParam(r, "propertyId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}

		var body gen.PropertyUpdate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		body.Name = identity.EasternToWesternDigits(body.Name)
		name := normalisePropertyName(body.Name)
		if n := len([]rune(name)); n < 2 || n > 120 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("name", MsgPropertyNameLength))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		if !s.lookupExists(r.Context(), tx, "property_type", body.PropertyTypeId) {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("propertyTypeId", MsgPropertyTypeRequired))
			return
		}
		if !s.lookupExists(r.Context(), tx, "area", body.AreaId) {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("areaId", MsgPropertyAreaRequired))
			return
		}

		// The prior state MUST be read before the update. Snapshotting the
		// returned row gives the new values for both sides of the audit
		// entry, which makes the record say nothing changed and makes undo
		// restore the value it was meant to replace.
		prior, err := s.properties.FindByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if prior == nil {
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound))
			return
		}

		nameNorm := identity.Canonical(name)
		updated, err := s.properties.UpdateProperty(r.Context(), tx,
			id, name, nameNorm, body.PropertyTypeId, body.AreaId,
			c.Account.ID, body.Version)
		if err != nil {
			switch err {
			case properties.ErrArchived:
				WriteError(w, NewAPIError(http.StatusConflict, CodeArchived, MsgArchived))
				return
			case properties.ErrVersionConflict:
				WriteError(w, NewAPIError(http.StatusConflict, CodeVersionConflict, MsgVersionConflict))
				return
			}
			refuseInternal(w, err)
			return
		}

		// Read custom values before replacing them so the audit can retain
		// only the fields that actually changed. Sensitive plaintext is used
		// in memory for comparison only and never enters the audit payload.
		previousValues, err := s.properties.ReadCustomValues(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		beforeState, err := newPropertyAuditState(prior, previousValues, s.envelope)
		if err != nil {
			refuseInternal(w, err)
			return
		}

		if err := s.properties.DeleteCustomValues(r.Context(), tx, id); err != nil {
			refuseInternal(w, err)
			return
		}
		customValues := customValueBodies(body.CustomValues)
		if len(customValues) > 0 {
			defs, err := s.loadCustomValueDefinitions(r.Context(), tx, customValues)
			if err != nil {
				refuseInternal(w, err)
				return
			}
			inputs, apiErr := buildCustomValueInputs(customValues, defs)
			if apiErr != nil {
				WriteError(w, apiErr)
				return
			}
			if err := s.properties.SaveCustomValues(r.Context(), tx, s.envelope, id, inputs); err != nil {
				refuseInternal(w, err)
				return
			}
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		afterValues, err := s.properties.ReadCustomValues(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		afterState, err := newPropertyAuditState(updated, afterValues, s.envelope)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		before, after := diffPropertyAuditStates(beforeState, afterState)
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.PropertyModified,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       updated.Code,
			SourceIP:       ClientIP(r),
			Before:         before,
			After:          after,
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		full, err := s.fetchFullProperty(r.Context(), id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(full)
	})
}

// handleChangePropertyCode implements PATCH /properties/{id}/code.
func (s *Server) handleChangePropertyCode() http.Handler {
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
		if c.Account.Role != "admin" {
			WriteError(w, NewAPIError(http.StatusForbidden, CodeForbidden, MsgForbidden))
			return
		}
		id, err := parseUUIDParam(r, "propertyId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}

		var body gen.ChangePropertyCodeJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		raw := strings.TrimSpace(identity.EasternToWesternDigits(body.Code))
		if n := len([]rune(raw)); n < 1 || n > 32 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("code", MsgPropertyCodeLength))
			return
		}
		codeNorm := identity.Canonical(raw)

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		oldCode, _, err := s.properties.ChangeCode(r.Context(), tx, id, raw, codeNorm, c.Account.ID, body.Version)
		if err != nil {
			switch err {
			case properties.ErrArchived:
				WriteError(w, NewAPIError(http.StatusConflict, CodeArchived, MsgArchived))
				return
			case properties.ErrVersionConflict:
				WriteError(w, NewAPIError(http.StatusConflict, CodeVersionConflict, MsgVersionConflict))
				return
			case properties.ErrDuplicateCode:
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgPropertyCodeTaken).
					WithField("code", MsgPropertyCodeTaken))
				return
			}
			refuseInternal(w, err)
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		before := map[string]any{"code": oldCode}
		after := map[string]any{"code": raw}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.PropertyCodeChanged,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       raw,
			SourceIP:       ClientIP(r),
			Before:         before,
			After:          after,
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		full, err := s.fetchFullProperty(r.Context(), id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(full)
	})
}

// handleArchiveProperty implements POST /properties/{id}/archive.
func (s *Server) handleArchiveProperty() http.Handler {
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
		id, err := parseUUIDParam(r, "propertyId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}
		var body gen.ArchivePropertyJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		var notePtr *string
		if body.Note != nil {
			trimmed := strings.TrimSpace(*body.Note)
			if trimmed == "" {
				body.Note = nil
			} else {
				if n := len([]rune(trimmed)); n > 500 {
					WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
						WithField("note", MsgPropertyArchiveNoteLength))
					return
				}
				note := trimmed
				notePtr = &note
			}
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		previous, err := s.properties.FindByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if previous == nil {
			_ = tx.Rollback(r.Context())
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound))
			return
		}

		p, err := s.properties.Archive(r.Context(), tx, id, c.Account.ID, body.Version, notePtr)
		if err != nil {
			switch err {
			case properties.ErrArchived:
				WriteError(w, NewAPIError(http.StatusConflict, CodeArchived, MsgArchived))
				return
			case properties.ErrVersionConflict:
				WriteError(w, NewAPIError(http.StatusConflict, CodeVersionConflict, MsgVersionConflict))
				return
			}
			refuseInternal(w, err)
			return
		}
		actorID := c.Account.ID
		actorRole := c.Account.Role
		before := map[string]any{"isArchived": false, "archiveNote": nil}
		after := map[string]any{"isArchived": true, "archiveNote": nil}
		if p.ArchiveNote != nil {
			after["archiveNote"] = *p.ArchiveNote
		}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.PropertyArchived,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       p.Code,
			SourceIP:       ClientIP(r),
			Before:         before,
			After:          after,
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		full, err := s.fetchFullProperty(r.Context(), id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(full)
	})
}

// handleRestoreProperty implements POST /properties/{id}/restore.
func (s *Server) handleRestoreProperty() http.Handler {
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
		id, err := parseUUIDParam(r, "propertyId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}
		var body gen.RestorePropertyJSONBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		previous, err := s.properties.FindByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if previous == nil {
			_ = tx.Rollback(r.Context())
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound))
			return
		}

		p, err := s.properties.Restore(r.Context(), tx, id, c.Account.ID, body.Version)
		if err != nil {
			switch err {
			case properties.ErrArchived:
				WriteError(w, NewAPIError(http.StatusConflict, CodeArchived, MsgArchived))
				return
			case properties.ErrVersionConflict:
				WriteError(w, NewAPIError(http.StatusConflict, CodeVersionConflict, MsgVersionConflict))
				return
			}
			refuseInternal(w, err)
			return
		}
		actorID := c.Account.ID
		actorRole := c.Account.Role
		before := map[string]any{"isArchived": true, "archiveNote": nil}
		if previous.ArchiveNote != nil {
			before["archiveNote"] = *previous.ArchiveNote
		}
		after := map[string]any{"isArchived": false, "archiveNote": nil}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.PropertyRestored,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       p.Code,
			SourceIP:       ClientIP(r),
			Before:         before,
			After:          after,
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		full, err := s.fetchFullProperty(r.Context(), id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(full)
	})
}

// fetchFullProperty returns the JSON response for one property including its
// custom values. Reads after the write transaction has committed.
func (s *Server) fetchFullProperty(ctx context.Context, id uuid.UUID) (map[string]any, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	p, err := s.properties.FindByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, properties.ErrNotFound
	}
	values, err := s.properties.ReadCustomValues(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	out := propertyResponse(p)
	out["customValues"] = customValuesResponse(values, s.envelope, ctx)
	return out, nil
}

// lookupExists returns true if a row with id exists in the named table.
func (s *Server) lookupExists(ctx context.Context, tx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, table string, id uuid.UUID) bool {
	row := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id = $1)`, id)
	var found bool
	if err := row.Scan(&found); err != nil {
		return false
	}
	return found
}
