package httpx

import (
	"encoding/json"
	"net/http"

	"pms/internal/properties"

	"github.com/google/uuid"
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

		var body genAdvancedSearch
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}

		in := properties.SearchInputs{
			Q:               derefString(body.Q),
			IncludeArchived: derefBool(body.IncludeArchived),
			Page:            derefInt(body.Page, 1),
			PageSize:        derefIntSearch(body.PageSize, 25),
		}
		if body.PropertyTypeID != nil {
			id := *body.PropertyTypeID
			in.PropertyTypeID = &id
		}
		if body.AreaID != nil {
			id := *body.AreaID
			in.AreaID = &id
		}
		for _, f := range body.CustomFilters {
			fid, err := uuid.Parse(f.FieldID)
			if err != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSearchUnknownField))
				return
			}
			sf := properties.SearchFilter{
				FieldID:  fid,
				Operator: properties.Operator(f.Operator),
			}
			if f.Text != nil {
				t := *f.Text
				sf.Text = &t
			}
			if f.ChoiceID != nil {
				id, err := uuid.Parse(*f.ChoiceID)
				if err != nil {
					WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSearchInvalidValue))
					return
				}
				sf.ChoiceID = &id
			}
			for _, raw := range f.ChoiceIDs {
				id, err := uuid.Parse(raw)
				if err != nil {
					WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSearchInvalidValue))
					return
				}
				sf.ChoiceIDs = append(sf.ChoiceIDs, id)
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

// genAdvancedSearch mirrors the shape the generated client sends. Keeping the
// field names aligned with the contract (Constitution III).
type genAdvancedSearch struct {
	Q               *string               `json:"q"`
	PropertyTypeID  *uuid.UUID            `json:"propertyTypeId"`
	AreaID          *uuid.UUID            `json:"areaId"`
	IncludeArchived *bool                 `json:"includeArchived"`
	Page            *int                  `json:"page"`
	PageSize        *int                  `json:"pageSize"`
	CustomFilters   []genCustomFilterBody `json:"customFilters"`
}

type genCustomFilterBody struct {
	FieldID   string   `json:"fieldId"`
	Operator  string   `json:"operator"`
	Text      *string  `json:"text"`
	ChoiceID  *string  `json:"choiceId"`
	ChoiceIDs []string `json:"choiceIds"`
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
