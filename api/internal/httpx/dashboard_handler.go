package httpx

import (
	"encoding/json"
	"net/http"

	"pms/internal/gen"
)

func (s *Server) handleDashboard() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, err := s.authenticate(w, r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if account == nil {
			WriteError(w, NewAPIError(http.StatusUnauthorized, CodeNotAuthenticated, MsgUnauthorized))
			return
		}

		var counts gen.DashboardCounts
		err = s.pool.QueryRow(r.Context(), `
			SELECT
				(SELECT count(*) FROM property),
				(SELECT count(*) FROM property WHERE archived_at IS NULL),
				(SELECT count(*) FROM property WHERE archived_at IS NOT NULL),
				(SELECT count(*) FROM attachment),
				(SELECT count(*) FROM attachment WHERE extract_state = 'pending'),
				(SELECT count(*) FROM custom_field),
				(SELECT count(*) FROM property_type),
				(SELECT count(*) FROM area)
		`).Scan(
			&counts.PropertiesTotal,
			&counts.PropertiesActive,
			&counts.PropertiesArchived,
			&counts.AttachmentsTotal,
			&counts.AttachmentsPending,
			&counts.CustomFieldsTotal,
			&counts.PropertyTypesTotal,
			&counts.AreasTotal,
		)
		if err != nil {
			refuseInternal(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(counts)
	})
}
