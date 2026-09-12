package httpx

import (
	"encoding/json"
	"net/http"

	"pms/internal/gen"
	"pms/internal/properties"
)

func (s *Server) handleSearchProperties() http.Handler {
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

		var body gen.AdvancedSearch
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}

		in := properties.SearchInputs{
			Q:               derefString(body.Q),
			DocumentText:    derefString(body.DocumentText),
			AttachmentName:  derefString(body.AttachmentName),
			HasAttachments:  derefHasAttachments(body.HasAttachments),
			IncludeArchived: derefBool(body.IncludeArchived),
			Page:            derefInt(body.Page, 1),
			PageSize:        derefPageSize(body.PageSize, 25),
		}
		if body.PropertyTypeId != nil {
			id := *body.PropertyTypeId
			in.PropertyTypeID = &id
		}
		if body.AreaId != nil {
			id := *body.AreaId
			in.AreaID = &id
		}
		// The generated types already carry parsed UUIDs, so nothing here
		// re-parses strings. Whether a field id is known, is sensitive, or
		// suits its operator is decided in the store, against the definitions
		// (research.md D-006) — never here.
		for _, f := range derefFilters(body.CustomFilters) {
			sf := properties.SearchFilter{
				FieldID:  f.FieldId,
				Operator: properties.Operator(f.Operator),
			}
			if f.Text != nil {
				t := *f.Text
				sf.Text = &t
			}
			if f.ChoiceId != nil {
				id := *f.ChoiceId
				sf.ChoiceID = &id
			}
			if f.ChoiceIds != nil {
				sf.ChoiceIDs = append(sf.ChoiceIDs, *f.ChoiceIds...)
			}
			in.CustomFilters = append(in.CustomFilters, sf)
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		byIDs, err := s.properties.BuildSearchInputs(r.Context(), tx, in)
		if err != nil {
			switch err {
			case properties.ErrUnknownField:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSearchUnknownField))
				return
			case properties.ErrSensitiveField:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSearchSensitiveField))
				return
			case properties.ErrOperatorMismatch:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSearchOperatorMismatch))
				return
			}
			refuseInternal(w, err)
			return
		}

		items, total, err := s.properties.SearchProperties(r.Context(), tx, in, byIDs)
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
			"page":       in.Page,
			"pageSize":   in.PageSize,
			"totalItems": total,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// The request body is the generated contract type. Mirroring it by hand here
// would duplicate spec types, which Constitution III prohibits.

// derefFilters unwraps the generated optional slice.
func derefFilters(p *[]gen.CustomFieldFilter) []gen.CustomFieldFilter {
	if p == nil {
		return nil
	}
	return *p
}

// derefPageSize unwraps the generated page-size enum.
func derefPageSize(p *gen.AdvancedSearchPageSize, def int) int {
	if p == nil {
		return def
	}
	return int(*p)
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}
func derefInt(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}
func derefIntSearch(p *int, def int) int {
	if p == nil {
		return def
	}
	v := *p
	switch v {
	case 10, 25, 50, 100:
		return v
	default:
		return def
	}
}

// derefHasAttachments unwraps the generated optional enum, defaulting to the
// unconstrained case.
func derefHasAttachments(p *gen.AdvancedSearchHasAttachments) string {
	if p == nil {
		return "any"
	}
	return string(*p)
}
