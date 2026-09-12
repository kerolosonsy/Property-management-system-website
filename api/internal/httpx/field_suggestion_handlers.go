package httpx

import (
	"encoding/json"
	"net/http"

	"pms/internal/gen"
	"pms/internal/properties"

	"github.com/google/uuid"
)

// handleFieldSuggestions serves GET /properties/field-suggestions: type-ahead
// values for one search filter. A pure read — no audit row (Constitution VIII
// logs writes and attachment reads, not ordinary reads) — and a sensitive
// custom field is refused with 400 before any query, exactly as the advanced
// search refuses one. Nothing is decrypted here (Constitution VII).
func (s *Server) handleFieldSuggestions(field, fieldID, q string) http.Handler {
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

		var fid *uuid.UUID
		if fieldID != "" {
			id, perr := uuid.Parse(fieldID)
			if perr != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
				return
			}
			fid = &id
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		items, err := s.properties.FieldSuggestions(r.Context(), tx, field, fid, q)
		if err != nil {
			switch err {
			case properties.ErrSuggestionTarget:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSuggestionFieldRequired))
				return
			case properties.ErrUnknownField:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSearchUnknownField))
				return
			case properties.ErrSensitiveField:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSearchSensitiveField))
				return
			case properties.ErrCheckboxNoSuggestions:
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgSuggestionCheckboxField))
				return
			}
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(gen.FieldSuggestions{Suggestions: items})
	})
}
