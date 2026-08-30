package httpx

import (
	"fmt"
	"log/slog"
	"net/http"

	"pms/internal/auth"
	"pms/internal/config"
	pmscrypto "pms/internal/crypto"
	"pms/internal/identity"
	"pms/internal/properties"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Server bundles the dependencies the HTTP handlers need.
type Server struct {
	cfg       *config.Config
	pool      *pgxpool.Pool
	identity  *identity.Store
	properties *properties.Store
	envelope  *pmscrypto.Envelope
	delay     auth.DelaySchedule
}

// auditService is unused at the type level; audit rows are written inline by
// handlers using audit.Write against the same transaction as the change.

func NewServer(
	cfg *config.Config, pool *pgxpool.Pool, store *identity.Store,
	props *properties.Store, env *pmscrypto.Envelope,
) *Server {
	return &Server{
		cfg:       cfg,
		pool:      pool,
		identity:  store,
		properties: props,
		envelope:  env,
		delay:     auth.DelaySchedule{},
	}
}

// Routes returns the application's http.Handler. The generated handler is
// responsible for dispatching requests to the right ServerInterface method;
// here we wrap each protected route with the role check (Principle I) and
// add a request log. The generated handler was created with BaseURL=/api/v1
// so its pattern registrations match the public URL space directly.
func (s *Server) Routes(genHandler http.Handler) http.Handler {
	mux := http.NewServeMux()

	// Unauthenticated.
	mux.Handle("GET /api/v1/health", s.handleHealth())
	mux.Handle("POST /api/v1/auth/login", s.handleSignIn())

	// Authenticated; any role.
	mux.Handle("POST /api/v1/auth/logout", s.authed(anyRole, genHandler.ServeHTTP))
	mux.Handle("GET /api/v1/auth/me", s.authed(anyRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/auth/password", s.authed(anyRole, genHandler.ServeHTTP))

	// Lookups — read by any signed-in user (filter bar needs them).
	mux.Handle("GET /api/v1/property-types", s.authed(anyRole, genHandler.ServeHTTP))
	mux.Handle("GET /api/v1/areas", s.authed(anyRole, genHandler.ServeHTTP))

	// Custom field definitions — read by any signed-in user.
	mux.Handle("GET /api/v1/custom-fields", s.authed(anyRole, genHandler.ServeHTTP))

	// Properties — read by manager, write by manager; PATCH /code is admin.
	mux.Handle("GET /api/v1/properties", s.authed(anyRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/properties", s.authed(anyRole, genHandler.ServeHTTP))
	// Search must be registered before the {propertyId} pattern (research D-006).
	mux.Handle("POST /api/v1/properties/search", s.authed(anyRole, genHandler.ServeHTTP))
	mux.Handle("GET /api/v1/properties/{propertyId}", s.authed(anyRole, genHandler.ServeHTTP))
	mux.Handle("PUT /api/v1/properties/{propertyId}", s.authed(anyRole, genHandler.ServeHTTP))
	mux.Handle("PATCH /api/v1/properties/{propertyId}/code", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/properties/{propertyId}/archive", s.authed(anyRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/properties/{propertyId}/restore", s.authed(anyRole, genHandler.ServeHTTP))

	// Administrator only — accounts, audit records, and the four
	// configuration write operations on lookups and custom fields.
	mux.Handle("GET /api/v1/users", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/users", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("GET /api/v1/users/{userId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("PATCH /api/v1/users/{userId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/users/{userId}/password", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("GET /api/v1/audit-records", s.authed(adminRole, genHandler.ServeHTTP))

	mux.Handle("POST /api/v1/property-types", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("PUT /api/v1/property-types/{lookupId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("DELETE /api/v1/property-types/{lookupId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/areas", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("PUT /api/v1/areas/{lookupId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("DELETE /api/v1/areas/{lookupId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/custom-fields", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("PUT /api/v1/custom-fields/{fieldId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("DELETE /api/v1/custom-fields/{fieldId}", s.authed(adminRole, genHandler.ServeHTTP))

	return logMiddleware(mux)
}

const (
	anyRole   = "any"
	adminRole = "admin"
)

func (s *Server) authed(role string, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := s.authenticate(w, r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if role == adminRole {
			if err := requireRole(c, "admin"); err != nil {
				WriteError(w, err)
				return
			}
		}
		// FR-007: while a password change is outstanding no other action is
		// possible. Three calls stay reachable because they are how the user
		// resolves or abandons that state: reading their own identity, changing
		// the password, and signing out.
		if c != nil && c.Account.MustChangePassword && !passwordChangeExempt(r) {
			WriteError(w, NewAPIError(http.StatusForbidden, CodePasswordChangeRequired, MsgPasswordChangeRequired))
			return
		}
		next(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"ip", ClientIP(r),
		)
		next.ServeHTTP(w, r)
	})
}

// refuseNotFound is a helper for handlers returning 404.
func refuseNotFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgNotFound))
}

// refuseInternal hides detail (FR-028).
func refuseInternal(w http.ResponseWriter, err error) {
	slog.Error("internal error", "err", err, "err_type", fmt.Sprintf("%T", err))
	WriteError(w, NewAPIError(http.StatusInternalServerError, CodeInternalError, MsgInternalError))
}

// passwordChangeExempt names the only endpoints reachable while an account has
// an outstanding forced password change. Everything else is refused by FR-007.
// /auth/me is exempt because it is how the client discovers the state at all.
func passwordChangeExempt(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/auth/me", "/api/v1/auth/password", "/api/v1/auth/logout":
		return true
	default:
		return false
	}
}
