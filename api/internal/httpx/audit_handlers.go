package httpx

import (
	"encoding/json"
	"net/http"
	"time"

	"pms/internal/audit"

	"github.com/google/uuid"
)

func (s *Server) handleListAuditRecords() http.Handler {
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

		page := atoiDefault(r.URL.Query().Get("page"), 1)
		pageSize := atoiDefault(r.URL.Query().Get("pageSize"), 25)
		if pageSize < 1 || pageSize > 100 {
			pageSize = 25
		}
		if page < 1 {
			page = 1
		}

		f := audit.Filters{Page: page, PageSize: pageSize}

		if v := r.URL.Query().Get("actorId"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
				return
			}
			f.ActorID = &id
		}
		if v := r.URL.Query().Get("targetId"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
				return
			}
			f.TargetID = &id
		}
		if v := r.URL.Query().Get("action"); v != "" {
			act := audit.Action(v)
			f.Action = &act
		}

		// Date-range filters are inclusive of the whole day in Africa/Cairo
		// (FR-047). Cairo is UTC+2 year-round (no DST), so "whole day in Cairo"
		// maps to [date 00:00+02, date+1 00:00+02) which equals
		// [date-1d 22:00Z, date+1d-1ns 22:00Z) in UTC.
		if from := r.URL.Query().Get("from"); from != "" {
			t, err := parseCairoDay(from, false)
			if err != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
				return
			}
			f.From = &t
		}
		if to := r.URL.Query().Get("to"); to != "" {
			t, err := parseCairoDay(to, true)
			if err != nil {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
				return
			}
			f.To = &t
		}
		if f.From != nil && f.To != nil && f.From.After(*f.To) {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("from", MsgInvalidDateRange))
			return
		}

		items, total, err := audit.Query(r.Context(), s.pool, f)
		if err != nil {
			refuseInternal(w, err)
			return
		}

		respItems := make([]map[string]any, 0, len(items))
		for _, r := range items {
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
			respItems = append(respItems, rec)
		}

		resp := map[string]any{
			"items":      respItems,
			"page":       page,
			"pageSize":   pageSize,
			"totalItems": total,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// parseCairoDay expands a YYYY-MM-DD to a UTC time that represents either the
// start of that day in Africa/Cairo (inclusive=false) or the end of that day
// (inclusive=true).
func parseCairoDay(s string, endOfDay bool) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, err
	}
	// Cairo is UTC+2 with no DST.
	offset := 2 * time.Hour
	if endOfDay {
		// End of day in Cairo = 23:59:59.999999999 local = next-day 21:59:59.999...Z.
		// For the inclusive upper bound the spec says the day is included at
		// both ends, so use next-day 00:00 local - 1ns (== 21:59:59.999999999Z).
		return t.Add(24 * time.Hour).Add(offset).Add(-time.Nanosecond), nil
	}
	return t.Add(offset), nil
}
