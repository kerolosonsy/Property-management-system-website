package httpx

import (
	"encoding/json"
	"net/http"

	"pms/internal/audit"
	"pms/internal/auth"
)

func (s *Server) handleSignOut() http.Handler {
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

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		if err := auth.RevokeSession(r.Context(), tx, c.SessionID); err != nil {
			refuseInternal(w, err)
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:        audit.SignOut,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			SourceIP:       ClientIP(r),
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		// Clear the cookie.
		http.SetCookie(w, &http.Cookie{
			Name:     s.cfg.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		})

		w.WriteHeader(http.StatusNoContent)
	})
}

func (s *Server) handleMe() http.Handler {
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
		// /auth/me answers 200 even when a password change is outstanding: the
		// contract declares only 200 and 401 here, and CurrentUser.mustChangePassword
		// exists precisely so the client can learn the state and route to the change
		// screen. Refusing this call would leave the client unable to discover who it
		// is, and therefore unable to reach that screen at all. FR-007 is enforced on
		// every other endpoint by requirePasswordChanged in the route wrapper.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(currentUserResponse(c.Account))
	})
}

type changePasswordBody struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Server) handleChangePassword() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body changePasswordBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		c, err := s.authenticate(w, r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if c == nil {
			WriteError(w, NewAPIError(http.StatusUnauthorized, CodeNotAuthenticated, MsgUnauthorized))
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

		// Re-fetch the account inside the transaction so we hash-verify against
		// the freshest stored hash.
		acct, err := s.identity.FindByID(r.Context(), tx, c.Account.ID)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if acct == nil {
			WriteError(w, NewAPIError(http.StatusUnauthorized, CodeNotAuthenticated, MsgUnauthorized))
			return
		}
		if err := auth.VerifyPassword(body.CurrentPassword, acct.PasswordHash); err != nil {
			WriteError(w, NewAPIError(http.StatusUnauthorized, CodeInvalidCredentials, MsgInvalidCredentials))
			return
		}

		newHash, err := auth.HashPassword(body.NewPassword)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if err := s.identity.UpdatePassword(r.Context(), tx, acct.ID, newHash, false); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := auth.RevokeAllForAccount(r.Context(), tx, acct.ID); err != nil {
			refuseInternal(w, err)
			return
		}

		actorID := acct.ID
		actorRole := acct.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:        audit.PasswordChanged,
			ActorAccountID: &actorID,
			ActorUsername:  acct.Username,
			ActorRole:      &actorRole,
			TargetAccountID: &actorID,
			TargetUsername:  strPtr(acct.Username),
			SourceIP:       ClientIP(r),
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:        audit.SessionsInvalidated,
			ActorAccountID: &actorID,
			ActorUsername:  acct.Username,
			ActorRole:      &actorRole,
			TargetAccountID: &actorID,
			TargetUsername:  strPtr(acct.Username),
			SourceIP:       ClientIP(r),
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		// Clear cookie (the current session was just revoked).
		http.SetCookie(w, &http.Cookie{
			Name:     s.cfg.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		})
		w.WriteHeader(http.StatusNoContent)
	})
}

func strPtr(s string) *string { return &s }
