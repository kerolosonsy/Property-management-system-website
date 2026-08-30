package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"pms/internal/audit"
	"pms/internal/auth"
	"pms/internal/identity"
)

// signInBody mirrors the OpenAPI request body. Using a private struct here
// keeps the handler independent of any future generated body type.
type signInBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleSignIn() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body signInBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}

		// Eastern-to-Western digits before validation (FR-033, FR-038).
		body.Username = identity.EasternToWesternDigits(body.Username)

		// Field-level validation (FR-036, FR-037).
		if err := validateUsername(body.Username); err != nil {
			WriteError(w, err)
			return
		}
		if body.Password == "" {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("password", MsgInvalidRequest))
			return
		}

		canonical := identity.Canonical(body.Username)
		ip := ClientIP(r)

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		// Progressive delay — applies equally to unknown usernames (FR-040).
		wait, err := s.delay.Consult(r.Context(), tx, canonical)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if wait > 0 {
			_ = tx.Commit(r.Context())
			apiErr := NewAPIError(http.StatusTooManyRequests, CodeTooSoon, MsgTooSoon).
				WithRetry(wait)
			WriteError(w, apiErr)
			return
		}

		acct, err := s.identity.FindByCanonical(r.Context(), tx, canonical)
		if err != nil {
			refuseInternal(w, err)
			return
		}

		// Identical refusal for unknown username, wrong password, deactivated account (FR-013).
		if acct == nil {
			_ = s.delay.RecordFailure(r.Context(), tx, canonical)
			if err := audit.Write(r.Context(), tx, audit.Entry{
				Action:        audit.SignInFailed,
				ActorUsername: body.Username,
				SourceIP:      ip,
				Detail:        map[string]string{"reason": "unknown_user"},
			}); err != nil {
				refuseInternal(w, err)
				return
			}
			if err := tx.Commit(r.Context()); err != nil {
				refuseInternal(w, err)
				return
			}
			WriteError(w, NewAPIError(http.StatusUnauthorized, CodeInvalidCredentials, MsgInvalidCredentials))
			return
		}

		if err := auth.VerifyPassword(body.Password, acct.PasswordHash); err != nil {
			_ = s.delay.RecordFailure(r.Context(), tx, canonical)
			if err := audit.Write(r.Context(), tx, audit.Entry{
				Action:        audit.SignInFailed,
				ActorAccountID: &acct.ID,
				ActorUsername:  acct.Username,
				ActorRole:      rolePtr(acct.Role),
				SourceIP:       ip,
				Detail:         map[string]string{"reason": "bad_password"},
			}); err != nil {
				refuseInternal(w, err)
				return
			}
			if err := tx.Commit(r.Context()); err != nil {
				refuseInternal(w, err)
				return
			}
			WriteError(w, NewAPIError(http.StatusUnauthorized, CodeInvalidCredentials, MsgInvalidCredentials))
			return
		}

		if !acct.IsActive {
			if err := audit.Write(r.Context(), tx, audit.Entry{
				Action:        audit.SignInFailed,
				ActorAccountID: &acct.ID,
				ActorUsername:  acct.Username,
				ActorRole:      rolePtr(acct.Role),
				SourceIP:       ip,
				Detail:         map[string]string{"reason": "deactivated"},
			}); err != nil {
				refuseInternal(w, err)
				return
			}
			if err := tx.Commit(r.Context()); err != nil {
				refuseInternal(w, err)
				return
			}
			WriteError(w, NewAPIError(http.StatusUnauthorized, CodeInvalidCredentials, MsgInvalidCredentials))
			return
		}

		// Success — issue session, reset delay counter.
		token, err := auth.IssueSession(r.Context(), tx, acct.ID)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		_ = s.delay.Reset(r.Context(), tx, canonical)

		actorID := acct.ID
		actorRole := acct.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:        audit.SignInSucceeded,
			ActorAccountID: &actorID,
			ActorUsername:  acct.Username,
			ActorRole:      &actorRole,
			SourceIP:       ip,
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		// Set cookie.
		http.SetCookie(w, &http.Cookie{
			Name:     s.cfg.SessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(currentUserResponse(acct))
	})
}

func rolePtr(s string) *string { return &s }

// validateUsername applies FR-036 and FR-037.
func validateUsername(raw string) error {
	if l := len([]rune(raw)); l < 3 || l > 32 {
		return NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
			WithField("username", MsgUsernameLength)
	}
	if identity.HasInvisibleOrBidi(raw) {
		return NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
			WithField("username", MsgUsernameHasInvisible)
	}
	for _, r := range raw {
		if !identity.AllowedUsernameRune(r) {
			return NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("username", MsgUsernameChars)
		}
	}
	return nil
}

func currentUserResponse(a *identity.Account) map[string]any {
	out := map[string]any{
		"id":                  a.ID.String(),
		"username":            a.Username,
		"displayName":         a.DisplayName,
		"role":                a.Role,
		"mustChangePassword":  a.MustChangePassword,
	}
	return out
}

// Avoid unused import warnings when handlers are introduced in later tasks.
var _ = errors.New
var _ = context.Background
