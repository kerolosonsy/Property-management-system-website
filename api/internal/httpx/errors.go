// Package httpx provides HTTP plumbing: the error envelope, the message catalogue,
// the source-IP extraction helper, and the middleware that authorizes each
// request. It also wires the generated route interface to concrete handlers.
package httpx

import (
	"encoding/json"
	"net/http"

	"pms/internal/gen"
)

// Error codes (must match contracts/openapi.yaml components.schemas.Error).
const (
	CodeInvalidRequest         = "invalid_request"
	CodeInvalidCredentials     = "invalid_credentials"
	CodeTooSoon                = "too_soon"
	CodeNotAuthenticated       = "not_authenticated"
	CodeForbidden              = "forbidden"
	CodeNotFound               = "not_found"
	CodeConflict               = "conflict"
	CodeInUse                  = "in_use"
	CodeVersionConflict        = "version_conflict"
	CodeArchived               = "archived"
	CodePasswordChangeRequired = "password_change_required"
	CodeInternalError          = "internal_error"
)

// APIError is a typed error that carries the code, message, status, and optional
// retry-after and field-level messages.
type APIError struct {
	Status   int
	Code     string
	Message  string
	Fields   map[string]string
	RetrySec int
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func NewAPIError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

func (e *APIError) WithRetry(sec int) *APIError {
	e.RetrySec = sec
	return e
}

func (e *APIError) WithField(field, msg string) *APIError {
	if e.Fields == nil {
		e.Fields = map[string]string{}
	}
	e.Fields[field] = msg
	return e
}

// WriteError serializes the error envelope. It is the only place a 5xx body is
// built; FR-028 forbids returning internal detail.
func WriteError(w http.ResponseWriter, err error) {
	apiErr, ok := err.(*APIError)
	if !ok {
		apiErr = NewAPIError(http.StatusInternalServerError, CodeInternalError, MsgInternalError)
	}
	body := gen.Error{
		Code:    gen.ErrorCode(apiErr.Code),
		Message: apiErr.Message,
	}
	if len(apiErr.Fields) > 0 {
		fields := make(map[string]string, len(apiErr.Fields))
		for k, v := range apiErr.Fields {
			fields[k] = v
		}
		body.Fields = &fields
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(apiErr.Status)
	if apiErr.RetrySec > 0 {
		// Add retryAfterSeconds as a sibling of Error.
		raw := map[string]any{
			"code":               body.Code,
			"message":            body.Message,
		}
		if body.Fields != nil {
			raw["fields"] = body.Fields
		}
		raw["retryAfterSeconds"] = apiErr.RetrySec
		_ = json.NewEncoder(w).Encode(raw)
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}
