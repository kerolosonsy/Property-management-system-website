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

// Properties
func (s *Server) ListProperties(w http.ResponseWriter, r *http.Request, params gen.ListPropertiesParams) {
	s.handleListProperties().ServeHTTP(w, r)
}
func (s *Server) CreateProperty(w http.ResponseWriter, r *http.Request) {
	s.handleCreateProperty().ServeHTTP(w, r)
}
func (s *Server) GetProperty(w http.ResponseWriter, r *http.Request, propertyId openapi_types.UUID) {
	s.handleGetProperty().ServeHTTP(w, r)
}
func (s *Server) UpdateProperty(w http.ResponseWriter, r *http.Request, propertyId openapi_types.UUID) {
	s.handleUpdateProperty().ServeHTTP(w, r)
}
func (s *Server) ChangePropertyCode(w http.ResponseWriter, r *http.Request, propertyId openapi_types.UUID) {
	s.handleChangePropertyCode().ServeHTTP(w, r)
}
func (s *Server) ArchiveProperty(w http.ResponseWriter, r *http.Request, propertyId openapi_types.UUID) {
	s.handleArchiveProperty().ServeHTTP(w, r)
}
func (s *Server) RestoreProperty(w http.ResponseWriter, r *http.Request, propertyId openapi_types.UUID) {
	s.handleRestoreProperty().ServeHTTP(w, r)
}

// Lookups
func (s *Server) ListPropertyTypes(w http.ResponseWriter, r *http.Request) {
	s.handleListPropertyTypes().ServeHTTP(w, r)
}
func (s *Server) CreatePropertyType(w http.ResponseWriter, r *http.Request) {
	s.handleCreatePropertyType().ServeHTTP(w, r)
}
func (s *Server) RenamePropertyType(w http.ResponseWriter, r *http.Request, lookupId openapi_types.UUID) {
	s.handleRenamePropertyType().ServeHTTP(w, r)
}
func (s *Server) DeletePropertyType(w http.ResponseWriter, r *http.Request, lookupId openapi_types.UUID) {
	s.handleDeletePropertyType().ServeHTTP(w, r)
}
func (s *Server) ListAreas(w http.ResponseWriter, r *http.Request) {
	s.handleListAreas().ServeHTTP(w, r)
}
func (s *Server) CreateArea(w http.ResponseWriter, r *http.Request) {
	s.handleCreateArea().ServeHTTP(w, r)
}
func (s *Server) RenameArea(w http.ResponseWriter, r *http.Request, lookupId openapi_types.UUID) {
	s.handleRenameArea().ServeHTTP(w, r)
}
func (s *Server) DeleteArea(w http.ResponseWriter, r *http.Request, lookupId openapi_types.UUID) {
	s.handleDeleteArea().ServeHTTP(w, r)
}

// Custom fields
func (s *Server) ListCustomFields(w http.ResponseWriter, r *http.Request) {
	s.handleListCustomFields().ServeHTTP(w, r)
}
func (s *Server) CreateCustomField(w http.ResponseWriter, r *http.Request) {
	s.handleCreateCustomField().ServeHTTP(w, r)
}
func (s *Server) UpdateCustomField(w http.ResponseWriter, r *http.Request, fieldId openapi_types.UUID) {
	s.handleUpdateCustomField().ServeHTTP(w, r)
}
func (s *Server) DeleteCustomField(w http.ResponseWriter, r *http.Request, fieldId openapi_types.UUID) {
	s.handleDeleteCustomField().ServeHTTP(w, r)
}

// Search
func (s *Server) SearchProperties(w http.ResponseWriter, r *http.Request) {
	s.handleSearchProperties().ServeHTTP(w, r)
}
