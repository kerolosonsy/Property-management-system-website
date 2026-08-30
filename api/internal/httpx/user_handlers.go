package httpx

import (
	"encoding/json"
	"net/http"
	"strconv"

	"pms/internal/audit"
	"pms/internal/auth"
	"pms/internal/identity"
)

type createUserBody struct {
	Username        string `json:"username"`
	DisplayName     string `json:"displayName"`
	Role            string `json:"role"`
	InitialPassword string `json:"initialPassword"`
}

func (s *Server) handleListUsers() http.Handler {
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
		includeInactive := true
		if v := r.URL.Query().Get("includeInactive"); v == "false" {
			includeInactive = false
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		items, total, err := s.identity.ListAccounts(r.Context(), tx, includeInactive, page, pageSize)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		_ = tx.Commit(r.Context())

		resp := map[string]any{
			"items":      toUsers(items),
			"page":       page,
			"pageSize":   pageSize,
			"totalItems": total,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func (s *Server) handleCreateUser() http.Handler {
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

		var body createUserBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}

		body.Username = identity.EasternToWesternDigits(body.Username)
		body.DisplayName = identity.EasternToWesternDigits(body.DisplayName)

		if err := validateUsername(body.Username); err != nil {
			WriteError(w, err)
			return
		}
		if l := len([]rune(body.DisplayName)); l < 1 || l > 120 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("displayName", MsgDisplayNameLength))
			return
		}
		if body.Role != "admin" && body.Role != "manager" {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("role", MsgRoleRequiredAdmin))
			return
		}
		if n := len([]rune(body.InitialPassword)); n < 12 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("initialPassword", MsgPasswordTooShort))
			return
		}

		hash, err := auth.HashPassword(body.InitialPassword)
		if err != nil {
			refuseInternal(w, err)
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		newAcct := &identity.Account{
			Username:          body.Username,
			UsernameCanonical: identity.Canonical(body.Username),
			DisplayName:       body.DisplayName,
			Role:              body.Role,
		}
		creatorID := c.Account.ID
		if err := s.identity.CreateAccount(r.Context(), tx, newAcct, hash, &creatorID); err != nil {
			if err == identity.ErrDuplicateUsername {
				_ = tx.Rollback(r.Context())
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgUsernameTaken).
					WithField("username", MsgUsernameTaken))
				return
			}
			refuseInternal(w, err)
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.AccountCreated,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			TargetAccountID: &newAcct.ID,
			TargetUsername:  strPtr(newAcct.Username),
			SourceIP:       ClientIP(r),
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
		_ = json.NewEncoder(w).Encode(toUser(*newAcct))
	})
}

func (s *Server) handleGetUser() http.Handler {
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
		id, err := parseUUIDParam(r, "userId")
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

		acct, err := s.identity.FindByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		_ = tx.Commit(r.Context())
		if acct == nil {
			refuseNotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(toUser(*acct))
	})
}

type updateUserBody struct {
	DisplayName *string `json:"displayName,omitempty"`
	Role        *string `json:"role,omitempty"`
	IsActive    *bool   `json:"isActive,omitempty"`
}

func (s *Server) handleUpdateUser() http.Handler {
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
		id, err := parseUUIDParam(r, "userId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}

		var body updateUserBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		if body.DisplayName == nil && body.Role == nil && body.IsActive == nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}
		if body.DisplayName != nil {
			*body.DisplayName = identity.EasternToWesternDigits(*body.DisplayName)
			if l := len([]rune(*body.DisplayName)); l < 1 || l > 120 {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
					WithField("displayName", MsgDisplayNameLength))
				return
			}
		}
		if body.Role != nil && *body.Role != "admin" && *body.Role != "manager" {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("role", MsgRoleRequiredAdmin))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		current, err := s.identity.FindByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if current == nil {
			_ = tx.Rollback(r.Context())
			refuseNotFound(w, r)
			return
		}

		// Determine effective post-change values for the invariant check.
		postRole := current.Role
		if body.Role != nil {
			postRole = *body.Role
		}
		postActive := current.IsActive
		if body.IsActive != nil {
			postActive = *body.IsActive
		}
		if err := identity.EnsureNotLastAdmin(r.Context(), tx, id, c.Account.ID, postRole, postActive); err != nil {
			_ = tx.Rollback(r.Context())
			switch err {
			case identity.ErrSelfTarget:
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgSelfTarget))
			case identity.ErrLastAdmin:
				WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgLastAdmin))
			default:
				refuseInternal(w, err)
			}
			return
		}

		if err := s.identity.UpdateAccount(r.Context(), tx, id, body.DisplayName, body.Role, body.IsActive); err != nil {
			refuseInternal(w, err)
			return
		}

		updated, err := s.identity.FindByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		if body.Role != nil && *body.Role != current.Role {
			if err := audit.Write(r.Context(), tx, audit.Entry{
				Action:         audit.AccountRoleChanged,
				ActorAccountID: &actorID,
				ActorUsername:  c.Account.Username,
				ActorRole:      &actorRole,
				TargetAccountID: &updated.ID,
				TargetUsername:  strPtr(updated.Username),
				SourceIP:       ClientIP(r),
				Detail: map[string]any{
					"from": current.Role,
					"to":   *body.Role,
				},
			}); err != nil {
				refuseInternal(w, err)
				return
			}
		}
		if body.IsActive != nil && *body.IsActive != current.IsActive {
			action := audit.AccountActivated
			if !*body.IsActive {
				action = audit.AccountDeactivated
			}
			if err := audit.Write(r.Context(), tx, audit.Entry{
				Action:         action,
				ActorAccountID: &actorID,
				ActorUsername:  c.Account.Username,
				ActorRole:      &actorRole,
				TargetAccountID: &updated.ID,
				TargetUsername:  strPtr(updated.Username),
				SourceIP:       ClientIP(r),
			}); err != nil {
				refuseInternal(w, err)
				return
			}
			// Deactivation revokes every session for the account (FR-020).
			if !*body.IsActive {
				if err := auth.RevokeAllForAccount(r.Context(), tx, updated.ID); err != nil {
					refuseInternal(w, err)
					return
				}
				if err := audit.Write(r.Context(), tx, audit.Entry{
					Action:         audit.SessionsInvalidated,
					ActorAccountID: &actorID,
					ActorUsername:  c.Account.Username,
					ActorRole:      &actorRole,
					TargetAccountID: &updated.ID,
					TargetUsername:  strPtr(updated.Username),
					SourceIP:       ClientIP(r),
				}); err != nil {
					refuseInternal(w, err)
					return
				}
			}
		}

		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		w.Header().Set("-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(toUser(*updated))
	})
}

type resetPasswordBody struct {
	NewPassword string `json:"newPassword"`
}

func (s *Server) handleResetUserPassword() http.Handler {
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
		id, err := parseUUIDParam(r, "userId")
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}

		var body resetPasswordBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		if n := len([]rune(body.NewPassword)); n < 12 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("newPassword", MsgPasswordTooShort))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		target, err := s.identity.FindByID(r.Context(), tx, id)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if target == nil {
			_ = tx.Rollback(r.Context())
			refuseNotFound(w, r)
			return
		}

		hash, err := auth.HashPassword(body.NewPassword)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if err := s.identity.UpdatePassword(r.Context(), tx, id, hash, true); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := auth.RevokeAllForAccount(r.Context(), tx, id); err != nil {
			refuseInternal(w, err)
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.PasswordReset,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			TargetAccountID: &target.ID,
			TargetUsername:  strPtr(target.Username),
			SourceIP:       ClientIP(r),
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.SessionsInvalidated,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			TargetAccountID: &target.ID,
			TargetUsername:  strPtr(target.Username),
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

func toUser(a identity.Account) map[string]any {
	out := map[string]any{
		"id":                 a.ID.String(),
		"username":           a.Username,
		"displayName":        a.DisplayName,
		"role":               a.Role,
		"isActive":           a.IsActive,
		"mustChangePassword": a.MustChangePassword,
		"createdAt":          a.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
	if !a.UpdatedAt.IsZero() {
		out["updatedAt"] = a.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	}
	return out
}

func toUsers(items []identity.Account) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		out = append(out, toUser(a))
	}
	return out
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
