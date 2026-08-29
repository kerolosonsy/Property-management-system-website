package httpx

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"pms/internal/auth"
	"pms/internal/identity"

	"github.com/google/uuid"
)

type ctxKey int

const (
	ctxAccountKey ctxKey = iota
)

// Account is the per-request snapshot of the signed-in account, read fresh
// from stored data on every request (FR-034). The session carries identity
// only; role and active state are NOT trusted from the session.
type CurrentAccount struct {
	Account *identity.Account
	SessionID uuid.UUID
}

// FromContext returns the request's current account, or nil if none.
func FromContext(ctx context.Context) *CurrentAccount {
	v, _ := ctx.Value(ctxAccountKey).(*CurrentAccount)
	return v
}

func putAccount(ctx context.Context, c *CurrentAccount) context.Context {
	return context.WithValue(ctx, ctxAccountKey, c)
}

// requireSession resolves the session cookie, looks up the session and the
// account, touches the session, and stores the account in the request context.
// Touching is idempotent and re-checks the limits so a request that races the
// idle boundary is rejected cleanly.
func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) (*CurrentAccount, error) {
	c, err := r.Cookie(s.cfg.SessionCookieName)
	if err != nil || c.Value == "" {
		return nil, nil
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())

	sess, err := auth.LookupSession(r.Context(), tx, c.Value)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		_ = tx.Commit(r.Context())
		return nil, nil
	}
	acct, err := s.identity.FindByID(r.Context(), tx, sess.AccountID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		_ = tx.Commit(r.Context())
		return nil, nil
	}
	if err := auth.TouchSession(r.Context(), tx, sess.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return nil, err
	}
	return &CurrentAccount{Account: acct, SessionID: sess.ID}, nil
}

// authenticate runs requireSession and either sets the context or refuses with
// not_authenticated. Used by handlers that require any signed-in user.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (*CurrentAccount, error) {
	c, err := s.requireSession(w, r)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, NewAPIError(http.StatusUnauthorized, CodeNotAuthenticated, MsgUnauthorized)
	}
	if !c.Account.IsActive {
		// Deactivated between issuing the cookie and now — refuse (Edge Cases).
		return nil, NewAPIError(http.StatusUnauthorized, CodeNotAuthenticated, MsgUnauthorized)
	}
	// Replace request context with the resolved account so handlers see it.
	*r = *r.WithContext(putAccount(r.Context(), c))
	return c, nil
}

func requireRole(c *CurrentAccount, role string) error {
	if c == nil {
		return NewAPIError(http.StatusUnauthorized, CodeNotAuthenticated, MsgUnauthorized)
	}
	if c.Account.Role != role {
		return NewAPIError(http.StatusForbidden, CodeForbidden, MsgForbidden)
	}
	return nil
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	v := strings.TrimSpace(r.PathValue(name))
	if v == "" {
		return uuid.Nil, errors.New("missing " + name)
	}
	return uuid.Parse(v)
}
