package httpx

import (
	"net/http"

	"pms/internal/gen"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Compile-time assertion that *Server satisfies gen.ServerInterface.
var _ gen.ServerInterface = (*Server)(nil)

// The methods below satisfy gen.ServerInterface by dispatching into the
// concrete handlers defined elsewhere. Each method takes the exact parameters
// the generated interface expects, then delegates to the handler that holds the
// real logic.

func (s *Server) GetHealth(w http.ResponseWriter, r *http.Request) {
	s.handleHealth().ServeHTTP(w, r)
}

func (s *Server) SignIn(w http.ResponseWriter, r *http.Request) {
	s.handleSignIn().ServeHTTP(w, r)
}

func (s *Server) SignOut(w http.ResponseWriter, r *http.Request) {
	s.handleSignOut().ServeHTTP(w, r)
}

func (s *Server) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	s.handleMe().ServeHTTP(w, r)
}

func (s *Server) ChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	s.handleChangePassword().ServeHTTP(w, r)
}

func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request, params gen.ListUsersParams) {
	s.handleListUsers().ServeHTTP(w, r)
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	s.handleCreateUser().ServeHTTP(w, r)
}

func (s *Server) GetUser(w http.ResponseWriter, r *http.Request, userId openapi_types.UUID) {
	s.handleGetUser().ServeHTTP(w, r)
}

func (s *Server) UpdateUser(w http.ResponseWriter, r *http.Request, userId openapi_types.UUID) {
	s.handleUpdateUser().ServeHTTP(w, r)
}

func (s *Server) ResetUserPassword(w http.ResponseWriter, r *http.Request, userId openapi_types.UUID) {
	s.handleResetUserPassword().ServeHTTP(w, r)
}

func (s *Server) ListAuditRecords(w http.ResponseWriter, r *http.Request, params gen.ListAuditRecordsParams) {
	s.handleListAuditRecords().ServeHTTP(w, r)
}
