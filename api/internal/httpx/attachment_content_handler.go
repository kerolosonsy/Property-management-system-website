// Attachment content handler — GET /attachments/{id}/content.
//
// Streams the decrypted bytes for one attachment, per request. The audit
// row for the read is written and committed BEFORE the first byte is
// served (research.md D-010, FR-013d). The response carries
// Cache-Control: no-store so the browser writes no decrypted copy to disk
// (research.md D-009).
//
// Integrity failures split by when they are found. The header and the first
// chunk are verified BEFORE the status line is written, so tampering there
// yields 422 with zero bytes (FR-014). Chunked GCM cannot do better than that:
// tampering in a later chunk is only discovered while streaming, when 200 has
// already been sent. Such a failure is logged and the response is deliberately
// abandoned mid-body so the client sees a broken transfer rather than a short
// file it might trust. It is never silently discarded.

package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"pms/internal/audit"
	"pms/internal/blobstore"
	pmscrypto "pms/internal/crypto"
	"pms/internal/identity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// handleGetAttachmentContent streams the body of one attachment. The
// disposition query parameter chooses between inline (PDF and images only)
// and attachment (everything else).
func (s *Server) handleGetAttachmentContent(attachmentID uuid.UUID, params genGetAttachmentContentParams) http.Handler {
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

		a, err := loadAttachmentRow(r.Context(), tx, attachmentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgAttachmentNotFound))
				return
			}
			refuseInternal(w, err)
			return
		}

		var wrappedDEK, wrapNonce, noncePrefix []byte
		var chunkSize int32
		if err := tx.QueryRow(r.Context(), `
			SELECT wrapped_dek, wrap_nonce, nonce_prefix, chunk_size
			FROM attachment WHERE id = $1
		`, attachmentID).Scan(&wrappedDEK, &wrapNonce, &noncePrefix, &chunkSize); err != nil {
			refuseInternal(w, err)
			return
		}
		_ = tx.Commit(r.Context())

		// Audit row for the read is written and committed BEFORE any byte
		// leaves (research.md D-010). The row exists whether the stream
		// later succeeds or fails mid-flight.
		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.WriteCommitted(r.Context(), s.pool, audit.Entry{
			Action:         audit.AttachmentRead,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       a.PropertyID.String(),
			SourceIP:       ClientIP(r),
			Detail: map[string]any{
				"attachmentId":     attachmentID.String(),
				"disposition":      string(params.Disposition),
				"originalFilename": a.OriginalFilename,
			},
		}); err != nil {
			refuseInternal(w, err)
			return
		}

		rc, err := s.blobstore.Open(attachmentID)
		if err != nil {
			if errors.Is(err, errBlobstoreNotFound) {
				WriteError(w, NewAPIError(http.StatusUnprocessableEntity, CodeIntegrityFailed, MsgAttachmentIntegrityFailed))
				return
			}
			refuseInternal(w, err)
			return
		}
		defer rc.Close()

		// The stored file begins with its own header. OpenStream expects src to
		// be positioned at the first ciphertext byte, so the header MUST be
		// consumed here — reading it as chunk 0 is an authentication failure.
		fileHdr, _, err := pmscrypto.ReadHeader(rc)
		if err != nil {
			WriteError(w, NewAPIError(http.StatusUnprocessableEntity, CodeIntegrityFailed, MsgAttachmentIntegrityFailed))
			return
		}

		// Cross-check the file against what the database says about it. A
		// mismatch means the stored file is not the one this row describes —
		// swapped, replaced, or corrupted — and it is caught here, before the
		// status line is written, so the answer can still be 422 with no body.
		if !bytes.Equal(fileHdr.WrappedDEK, wrappedDEK) ||
			!bytes.Equal(fileHdr.WrapNonce, wrapNonce) ||
			!bytes.Equal(fileHdr.NoncePrefix, noncePrefix) ||
			fileHdr.ChunkSize != uint32(chunkSize) ||
			fileHdr.PlaintextLength != uint64(a.ByteSize) {
			slog.Error("stored attachment does not match its database row",
				"attachment_id", attachmentID.String())
			WriteError(w, NewAPIError(http.StatusUnprocessableEntity, CodeIntegrityFailed, MsgAttachmentIntegrityFailed))
			return
		}

		hdr := fileHdr

		// Set headers BEFORE the body is written.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-Type", a.ContentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		disposition := "attachment"
		if params.Disposition == "inline" && canViewInline(a.ContentType) {
			disposition = "inline"
		}
		w.Header().Set("Content-Disposition", disposition+`; filename="`+sanitizeFilename(a.OriginalFilename)+`"`)
		w.Header().Set("Content-Length", itoaInt64(a.ByteSize))
		w.WriteHeader(http.StatusOK)

		if err := s.envelope.OpenStream(hdr, rc, w); err != nil {
			// The status line is already sent, so this cannot become a 422.
			// Log it — an unreadable stored file is exactly the event an
			// operator must find out about — and abandon the body without
			// completing Content-Length, which makes net/http close the
			// connection so the client cannot mistake a truncated document
			// for a whole one.
			slog.Error("attachment stream failed after headers were sent",
				"attachment_id", attachmentID.String(),
				"actor", c.Account.Username,
				"err", err)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			panic(http.ErrAbortHandler)
		}
	})
}

func sanitizeFilename(name string) string {
	if name == "" {
		return "attachment"
	}
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == '"' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "attachment"
	}
	return out
}

func itoaInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// handleGetAttachmentText returns the extracted text for one attachment.
// A sensitive attachment's text is decrypted for this request only, after
// the role check, and the decrypted text never reaches a log or an audit
// row (FR-024c).
func (s *Server) handleGetAttachmentText(attachmentID uuid.UUID) http.Handler {
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

		var isSensitive bool
		var bodyText sqlNullString
		var truncated bool
		var correctedBy sqlNullString
		var correctedAt, extractedAt sqlNullTime
		err = tx.QueryRow(r.Context(), `
			SELECT a.is_sensitive,
			       t.body,
			       -- LEFT JOIN: no text row yet means no truncation, not NULL.
			       COALESCE(t.truncated, false),
			       t.corrected_by, t.corrected_at, t.extracted_at
			FROM attachment a
			LEFT JOIN attachment_text t ON t.attachment_id = a.id
			WHERE a.id = $1
		`, attachmentID).Scan(&isSensitive, &bodyText, &truncated, &correctedBy, &correctedAt, &extractedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgAttachmentNotFound))
				return
			}
			refuseInternal(w, err)
			return
		}

		out := map[string]any{
			"attachmentId": attachmentID.String(),
			"truncated":    truncated,
			"isCorrected":  correctedAt.Valid,
		}

		switch {
		case bodyText.Valid:
			out["body"] = bodyText.String
		case isSensitive:
			var cipherBody, cipherNonce, wrappedDEK, wrapNonce []byte
			if err := tx.QueryRow(r.Context(), `
				SELECT cipher_body, cipher_nonce, wrapped_dek, wrap_nonce
				FROM attachment_text WHERE attachment_id = $1
			`, attachmentID).Scan(&cipherBody, &cipherNonce, &wrappedDEK, &wrapNonce); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				refuseInternal(w, err)
				return
			}
			if cipherBody != nil {
				plain, err := s.envelope.Open(&pmscrypto.Sealed{
					Ciphertext: cipherBody, CipherNonce: cipherNonce,
					WrappedDEK: wrappedDEK, WrapNonce: wrapNonce,
				})
				if err != nil {
					refuseInternal(w, err)
					return
				}
				out["body"] = string(plain)
			} else {
				out["body"] = nil
			}
		default:
			out["body"] = nil
		}

		if correctedBy.Valid {
			out["correctedBy"] = correctedBy.String
		} else {
			out["correctedBy"] = nil
		}
		if correctedAt.Valid {
			out["correctedAt"] = correctedAt.Time.UTC().Format(time.RFC3339Nano)
		} else {
			out["correctedAt"] = nil
		}
		if extractedAt.Valid {
			out["extractedAt"] = extractedAt.Time.UTC().Format(time.RFC3339Nano)
		} else {
			out["extractedAt"] = nil
		}

		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	})
}

// handleCorrectAttachmentText applies a hand correction to one attachment's
// extracted text. The correction is sealed when the attachment is sensitive,
// so readable text is never stored in that case (FR-024c).
func (s *Server) handleCorrectAttachmentText(attachmentID uuid.UUID) http.Handler {
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

		var body struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		bodyText := strings.TrimSpace(body.Body)

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		var isSensitive bool
		if err := tx.QueryRow(r.Context(),
			`SELECT is_sensitive FROM attachment WHERE id = $1`,
			attachmentID).Scan(&isSensitive); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgAttachmentNotFound))
				return
			}
			refuseInternal(w, err)
			return
		}

		var (
			bodyCol     any = nil
			bodyNormCol any = nil
			cipherCol   any = nil
			cipherNCol  any = nil
			dekCol      any = nil
			dekNCol     any = nil
		)
		if isSensitive {
			sealed, err := s.envelope.Seal([]byte(bodyText))
			if err != nil {
				refuseInternal(w, err)
				return
			}
			cipherCol = sealed.Ciphertext
			cipherNCol = sealed.CipherNonce
			dekCol = sealed.WrappedDEK
			dekNCol = sealed.WrapNonce
		} else {
			bodyCol = bodyText
			n := identity.Canonical(bodyText)
			bodyNormCol = n
		}

		if _, err := tx.Exec(r.Context(), `
			INSERT INTO attachment_text (
				attachment_id, body, body_normalized,
				cipher_body, cipher_nonce, wrapped_dek, wrap_nonce,
				truncated, corrected_at, corrected_by, extracted_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, false, now(), $8, now())
			ON CONFLICT (attachment_id) DO UPDATE SET
				body = EXCLUDED.body,
				body_normalized = EXCLUDED.body_normalized,
				cipher_body = EXCLUDED.cipher_body,
				cipher_nonce = EXCLUDED.cipher_nonce,
				wrapped_dek = EXCLUDED.wrapped_dek,
				wrap_nonce = EXCLUDED.wrap_nonce,
				truncated = false,
				corrected_at = now(),
				corrected_by = $8,
				extracted_at = now()
		`,
			attachmentID, bodyCol, bodyNormCol,
			cipherCol, cipherNCol, dekCol, dekNCol,
			c.Account.ID); err != nil {
			refuseInternal(w, err)
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.AttachmentTextCorrected,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       attachmentID.String(),
			SourceIP:       ClientIP(r),
			Detail: map[string]any{
				"attachmentId": attachmentID.String(),
				"isSensitive":  isSensitive,
				"length":       len([]rune(bodyText)),
			},
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"attachmentId": attachmentID.String(),
			"truncated":    false,
			"isCorrected":  true,
		})
	})
}

// handleReextractAttachmentText re-queues an attachment for extraction.
func (s *Server) handleReextractAttachmentText(attachmentID uuid.UUID) http.Handler {
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

		var state string
		var correctedAt sqlNullTime
		if err := tx.QueryRow(r.Context(),
			`SELECT a.extract_state, t.corrected_at
			 FROM attachment a
			 LEFT JOIN attachment_text t ON t.attachment_id = a.id
			 WHERE a.id = $1`,
			attachmentID).Scan(&state, &correctedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgAttachmentNotFound))
				return
			}
			refuseInternal(w, err)
			return
		}
		if correctedAt.Valid {
			WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgAttachmentReextractCorrected))
			return
		}
		// Every terminal state may be retried. `done` is included deliberately:
		// extraction can succeed and still be wrong, and refusing to retry it
		// leaves the document permanently unsearchable. `pending` is the only
		// state that refuses, because an extraction is already under way.
		if state == "pending" {
			WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgAttachmentReextractInvalidState))
			return
		}

		tag, err := tx.Exec(r.Context(), `
			UPDATE attachment
			SET extract_state = 'pending', extract_attempts = 0,
			    extract_next_at = now(), extract_error = NULL
			WHERE id = $1 AND extract_state IN ('failed','empty','done','not_eligible','too_large')
		`, attachmentID)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if tag.RowsAffected() == 0 {
			WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgAttachmentReextractRunning))
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.AttachmentReextractRequested,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       attachmentID.String(),
			SourceIP:       ClientIP(r),
			Detail: map[string]any{
				"attachmentId": attachmentID.String(),
				"priorState":   state,
			},
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

// handleSearchDocuments implements POST /attachments/search (US5).
func (s *Server) handleSearchDocuments() http.Handler {
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

		var body struct {
			Q               string `json:"q"`
			IncludeArchived bool   `json:"includeArchived"`
			Page            int    `json:"page"`
			PageSize        int    `json:"pageSize"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
			return
		}
		q := strings.TrimSpace(body.Q)
		if n := len([]rune(q)); n < 2 || n > 200 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}
		if body.Page < 1 {
			body.Page = 1
		}
		if body.PageSize < 1 {
			body.PageSize = 25
		}
		switch body.PageSize {
		case 10, 25, 50, 100:
		default:
			body.PageSize = 25
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		qNorm := identity.CanonicalQuery(q)
		rows, err := tx.Query(r.Context(), `
			SELECT a.id, a.property_id, a.description, p.code, p.name
			FROM attachment_text t
			JOIN attachment a ON a.id = t.attachment_id
			JOIN property p ON p.id = a.property_id
			WHERE t.body_normalized LIKE $1
			  AND ($2 OR p.archived_at IS NULL)
			ORDER BY a.created_at DESC
			LIMIT $3 OFFSET $4
		`, "%"+qNorm+"%", body.IncludeArchived, body.PageSize, (body.Page-1)*body.PageSize)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer rows.Close()
		hits := []map[string]any{}
		for rows.Next() {
			var id, pid uuid.UUID
			var desc, code, name string
			if err := rows.Scan(&id, &pid, &desc, &code, &name); err != nil {
				refuseInternal(w, err)
				return
			}
			hits = append(hits, map[string]any{
				"propertyId":            pid.String(),
				"propertyCode":          code,
				"propertyName":          name,
				"attachmentId":          id.String(),
				"attachmentDescription": desc,
			})
		}
		if err := rows.Err(); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":             hits,
			"page":              body.Page,
			"pageSize":          body.PageSize,
			"totalItems":        len(hits),
			"sensitiveExcluded": true,
		})
	})
}

// genGetAttachmentContentParams is the local alias for the generated type.
type genGetAttachmentContentParams = struct {
	Disposition string
}

// errBlobstoreNotFound mirrors blobstore.ErrNotFound. Re-exported under a
// short name for use in errors.Is comparisons.
var errBlobstoreNotFound = blobstore.ErrNotFound
