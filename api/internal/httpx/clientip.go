package httpx

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the originating IP address for the request, in the form
// PostgreSQL's inet column accepts. Honours X-Forwarded-For when present so the
// audit trail records the network address the request came from (FR-023).
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return strings.TrimSpace(rip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
