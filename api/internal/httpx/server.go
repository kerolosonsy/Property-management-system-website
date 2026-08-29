package httpx

import (
	"fmt"
	"log/slog"
	"net/http"

	"pms/internal/auth"
	"pms/internal/config"
	"pms/internal/identity"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Server bundles the dependencies the HTTP handlers need.
type Server struct {
	cfg      *config.Config
	pool     *pgxpool.Pool
	identity *identity.Store
	delay    auth.DelaySchedule
}

// auditService is unused at the type level; audit rows are written inline by
// handlers using audit.Write against the same transaction as the change.

func NewServer(cfg *config.Config, pool *pgxpool.Pool, store *identity.Store) *Server {
	return &Server{
		cfg:      cfg,
		pool:     pool,
		identity: store,
		delay:    auth.DelaySchedule{},
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

	// Administrator only.
	mux.Handle("GET /api/v1/users", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/users", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("GET /api/v1/users/{userId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("PATCH /api/v1/users/{userId}", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("POST /api/v1/users/{userId}/password", s.authed(adminRole, genHandler.ServeHTTP))
	mux.Handle("GET /api/v1/audit-records", s.authed(adminRole, genHandler.ServeHTTP))

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
