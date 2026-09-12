package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"pms/internal/audit"
	"pms/internal/identity"
	"pms/internal/properties"

	"github.com/google/uuid"
	pmscrypto "pms/internal/crypto"
)

// propertyResponse shapes a single Property into the API's JSON form.
func propertyResponse(p *properties.Property) map[string]any {
	out := map[string]any{
		"id":           p.ID.String(),
		"code":         p.Code,
		"name":         p.Name,
		"propertyType": map[string]any{"id": p.PropertyTypeID.String(), "label": p.PropertyTypeLabel},
		"area":         map[string]any{"id": p.AreaID.String(), "label": p.AreaLabel},
		"version":      p.Version,
		"createdAt":    p.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"createdBy":    p.CreatedByName,
		"updatedAt":    p.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"updatedBy":    p.UpdatedByName,
		"isArchived":   p.ArchivedAt != nil,
		"archivedAt":   nil,
		"archivedBy":   nil,
		"archiveNote":  nil,
	}
	if p.ArchivedAt != nil {
		out["archivedAt"] = p.ArchivedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	}
	if p.ArchivedByName != nil {
		out["archivedBy"] = *p.ArchivedByName
	}
	if p.ArchiveNote != nil {
		out["archiveNote"] = *p.ArchiveNote
	}
	if len(p.MatchedAttachments) > 0 {
		matches := make([]map[string]any, 0, len(p.MatchedAttachments))
		for _, match := range p.MatchedAttachments {
			matches = append(matches, map[string]any{
				"id":          match.ID.String(),
				"description": match.Description,
			})
		}
		out["matchedAttachments"] = matches
	}
	return out
}

func propertyListResponse(items []properties.Property) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		out = append(out, propertyResponse(&items[i]))
	}
	return out
}

func customValuesResponse(values []properties.StoredValue, env *pmscrypto.Envelope, ctx context.Context) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, v := range values {
		entry := map[string]any{"fieldId": v.FieldID.String()}
		if v.IsMultiselect {
			ids := make([]string, 0, len(v.ChoiceIDs))
			for _, c := range v.ChoiceIDs {
				ids = append(ids, c.String())
			}
			entry["choiceIds"] = ids
			out = append(out, entry)
			continue
		}
		switch v.FieldType {
		case properties.FieldText:
			plain, _ := v.Decrypt(env)
			entry["text"] = plain
		case properties.FieldCheckbox:
			if v.Bool != nil {
				entry["checked"] = *v.Bool
			} else {
				entry["checked"] = nil
			}
		case properties.FieldDropdown:
			if v.ChoiceID != nil {
				entry["choiceId"] = v.ChoiceID.String()
			} else {
				entry["choiceId"] = nil
			}
		}
		out = append(out, entry)
	}
	return out
}

// readOptionalBool returns the named boolean query parameter or its default.
func readOptionalBool(r *http.Request, name string, def bool) bool {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func (s *Server) handleListProperties() http.Handler {
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

		page := atoiDefault(r.URL.Query().Get("page"), 1)
		pageSize := atoiDefaultPageSize(r.URL.Query().Get("pageSize"), 25)

		f := properties.ListFilters{
			Q:               r.URL.Query().Get("q"),
			Code:            r.URL.Query().Get("code"),
			Name:            r.URL.Query().Get("name"),
			IncludeArchived: readOptionalBool(r, "includeArchived", false),
			Page:            page,
			PageSize:        pageSize,
		}
		if v := r.URL.Query().Get("propertyTypeId"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
				return
			}
			f.PropertyTypeID = &id
		}
		if v := r.URL.Query().Get("areaId"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
				return
			}
			f.AreaID = &id
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		items, total, err := s.properties.ListProperties(r.Context(), tx, f)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		resp := map[string]any{
			"items":      propertyListResponse(items),
			"page":       page,
			"pageSize":   pageSize,
			"totalItems": total,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func (s *Server) handleGetProperty() http.Handler {
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

		id, err := parseUUIDParam(r, "propertyId")
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

		p, err := s.properties.FindByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if p == nil {
			_ = tx.Rollback(r.Context())
			WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound))
			return
		}
		values, err := s.properties.ReadCustomValues(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		out := propertyResponse(p)
		out["customValues"] = customValuesResponse(values, s.envelope, r.Context())
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	})
}

// pageSizeEnum enforces 10/25/50/100 per FR-009.
func atoiDefaultPageSize(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	switch v {
	case 10, 25, 50, 100:
		return v
	default:
		return def
	}
}

// Ensure imports stay even if helpers move around.
var _ = time.Now
var _ = identity.Canonical
var _ = audit.AccountCreated
