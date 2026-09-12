// Small shared helpers used by the attachment handlers.

package httpx

import (
	"database/sql"
	"fmt"
	"strings"
)

// sqlNullString is the type a column of TEXT NULL scans into.
type sqlNullString = sql.NullString

// sqlNullTime is the type a column of TIMESTAMPTZ NULL scans into.
type sqlNullTime = sql.NullTime

// formatTooLarge interpolates the configured limit into the Arabic too-large
// message. The template uses {limit} so the substitution is unambiguous and
// the rendered value is what the user actually sees.
func formatTooLarge(template string, limit int64) string {
	r := strings.NewReplacer("{limit}", fmt.Sprintf("%d", limit))
	return r.Replace(template)
}
