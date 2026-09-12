// Attachment HTTP handlers for feature 003-property-attachments-ocr.
//
// The handlers in this file are deliberately thin: they authenticate,
// read the request body, validate input, and delegate to the store. They
// never log or return decrypted bytes, never accept a path argument to a
// subprocess, and always check the audit write's error.
//
// Streaming an upload straight through the chunked encryptor into the
// store is the only correct way to satisfy "no plaintext byte on disk"
// (FR-011). The handler reads the multipart part's body as it streams and
// writes ciphertext to the store file; the file on disk is ciphertext
// from the very first byte.

package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"pms/internal/audit"
	pmscrypto "pms/internal/crypto"
	"pms/internal/extract"
	"pms/internal/identity"
	"pms/internal/properties"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const sniffHeadBytes = 8192

// attachmentResponse shapes one attachment for the API.
func attachmentResponse(a attachmentRow) map[string]any {
	out := map[string]any{
		"id":               a.ID.String(),
		"propertyId":       a.PropertyID.String(),
		"description":      a.Description,
		"originalFilename": a.OriginalFilename,
		"contentType":      a.ContentType,
		"byteSize":         a.ByteSize,
		"isSensitive":      a.IsSensitive,
		"extractState":     a.ExtractState,
		"canViewInline":    canViewInline(a.ContentType),
		"createdAt":        a.CreatedAt,
		"createdBy":        a.CreatedBy,
		"updatedAt":        a.UpdatedAt,
		"updatedBy":        a.UpdatedBy,
	}
	if a.ExtractError != nil {
		out["extractError"] = *a.ExtractError
	} else {
		out["extractError"] = nil
	}
	return out
}

// canViewInline mirrors the contract: PDF and images render in-browser;
// everything else is a download.
func canViewInline(contentType string) bool {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return true
	case contentType == "application/pdf":
		return true
	default:
		return false
	}
}

// attachmentRow is the in-process representation used by the handlers.
type attachmentRow struct {
	ID               uuid.UUID
	PropertyID       uuid.UUID
	Description      string
	OriginalFilename string
	ContentType      string
	ByteSize         int64
	IsSensitive      bool
	ExtractState     string
	ExtractError     *string
	CreatedAt        string
	CreatedBy        string
	UpdatedAt        string
	UpdatedBy        string
}

func formatTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z07:00") }

// handleListAttachments implements GET /properties/{id}/attachments.
func (s *Server) handleListAttachments(propertyID uuid.UUID) http.Handler {
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

		if err := requirePropertyExists(r.Context(), tx, propertyID); err != nil {
			if errors.Is(err, properties.ErrNotFound) {
				WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgPropertyNotFound))
				return
			}
			refuseInternal(w, err)
			return
		}

		rows, err := tx.Query(r.Context(), `
			SELECT a.id, a.property_id, a.description, a.original_filename,
			       a.content_type, a.byte_size, a.is_sensitive, a.extract_state,
			       a.extract_error, a.created_at, cu.username, a.updated_at, uu.username
			FROM attachment a
			JOIN account cu ON cu.id = a.created_by
			JOIN account uu ON uu.id = a.updated_by
			WHERE a.property_id = $1
			ORDER BY a.created_at DESC, a.id DESC
		`, propertyID)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var a attachmentRow
			var extractErr *string
			var createdAt, updatedAt time.Time
			if err := rows.Scan(&a.ID, &a.PropertyID, &a.Description, &a.OriginalFilename,
				&a.ContentType, &a.ByteSize, &a.IsSensitive, &a.ExtractState,
				&extractErr, &createdAt, &a.CreatedBy, &updatedAt, &a.UpdatedBy); err != nil {
				refuseInternal(w, err)
				return
			}
			a.ExtractError = extractErr
			a.CreatedAt = formatTime(createdAt)
			a.UpdatedAt = formatTime(updatedAt)
			out = append(out, attachmentResponse(a))
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
		_ = json.NewEncoder(w).Encode(out)
	})
}

// handleGetAttachment implements GET /attachments/{id}.
func (s *Server) handleGetAttachment(attachmentID uuid.UUID) http.Handler {
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
			if errors.Is(err, properties.ErrNotFound) {
				WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgAttachmentNotFound))
				return
			}
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(attachmentResponse(a))
	})
}

// errTooLarge is the sentinel the streaming code uses to recognise the
// configured limit being hit during the encrypt.
var errTooLarge = errors.New("attachment: too large")

// handleUploadAttachment implements POST /properties/{id}/attachments.
func (s *Server) handleUploadAttachment(propertyID uuid.UUID) http.Handler {
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

		// Reject early if Content-Length claims more than the cap. The
		// streaming code below also counts bytes as they arrive; this is
		// the cheap pre-check that avoids parsing a giant multipart.
		if r.ContentLength > 0 && r.ContentLength > s.cfg.AttachmentMaxBytes+65536 {
			WriteError(w, NewAPIError(http.StatusRequestEntityTooLarge, CodeTooLarge,
				formatTooLarge(MsgAttachmentTooLarge, s.cfg.AttachmentMaxBytes)))
			return
		}

		if err := r.ParseMultipartForm(s.cfg.AttachmentMaxBytes); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				WriteError(w, NewAPIError(http.StatusRequestEntityTooLarge, CodeTooLarge,
					formatTooLarge(MsgAttachmentTooLarge, s.cfg.AttachmentMaxBytes)))
				return
			}
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}

		description := strings.TrimSpace(r.FormValue("description"))
		if n := len([]rune(description)); n < 2 || n > 200 {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("description", MsgAttachmentDescriptionLength))
			return
		}
		isSensitive := r.FormValue("isSensitive") == "true"

		fileHeader, err := getMultipartFile(r)
		if err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest))
			return
		}

		f, err := fileHeader.Open()
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer f.Close()

		head := make([]byte, sniffHeadBytes)
		hn, herr := io.ReadFull(f, head)
		if herr != nil && herr != io.ErrUnexpectedEOF && herr != io.EOF {
			refuseInternal(w, herr)
			return
		}
		head = head[:hn]
		kind, contentType, err := extract.Sniff(bytes.NewReader(head))
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if !isAcceptedKind(kind) {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeUnsupportedType, MsgAttachmentUnsupportedType))
			return
		}

		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			refuseInternal(w, err)
			return
		}
		defer tx.Rollback(r.Context())

		archived, err := isPropertyArchived(r.Context(), tx, propertyID)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if archived {
			WriteError(w, NewAPIError(http.StatusConflict, CodeArchived, MsgAttachmentArchivedRefused))
			return
		}

		newID := uuid.New()
		storePath, err := s.blobstore.PathFor(newID)
		if err != nil {
			refuseInternal(w, err)
			return
		}
		if err := os.MkdirAll(parentDir(storePath), 0o755); err != nil {
			refuseInternal(w, err)
			return
		}
		storeFile, err := os.OpenFile(storePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			refuseInternal(w, err)
			return
		}

		combined := io.MultiReader(bytes.NewReader(head), f)
		cr := &countingReader{r: combined}
		// The declared length MUST be the real one: Stream is length-driven and
		// a zero here silently writes a header and no body. ParseMultipartForm
		// has already buffered the part, so Size is exact.
		hdr, err := s.envelope.Stream(cr, storeFile, fileHeader.Size)
		if closeErr := storeFile.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(storePath)
			refuseInternal(w, err)
			return
		}
		if cr.n > s.cfg.AttachmentMaxBytes {
			_ = os.Remove(storePath)
			WriteError(w, NewAPIError(http.StatusRequestEntityTooLarge, CodeTooLarge,
				formatTooLarge(MsgAttachmentTooLarge, s.cfg.AttachmentMaxBytes)))
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		var createdAt time.Time
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO attachment (
				id, property_id, description, original_filename, content_type, byte_size,
				description_normalized, filename_normalized,
				wrapped_dek, wrap_nonce, nonce_prefix, chunk_size,
				is_sensitive, extract_state,
				created_by, updated_by
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$13, $14,
				$7, $8, $9, $10,
				$11, 'pending',
				$12, $12
			)
			RETURNING created_at
		`,
			newID, propertyID, description, fileHeader.Filename, contentType, cr.n,
			hdr.WrappedDEK, hdr.WrapNonce, hdr.NoncePrefix, int32(hdr.ChunkSize),
			isSensitive, actorID,
			// Normalised forms for search, produced by the same normaliser as
			// property names so Arabic variants match identically.
			identity.Canonical(description), identity.Canonical(fileHeader.Filename),
		).Scan(&createdAt); err != nil {
			_ = os.Remove(storePath)
			refuseInternal(w, err)
			return
		}

		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.AttachmentAdded,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       propertyID.String(),
			SourceIP:       ClientIP(r),
			After: map[string]any{
				"attachmentId":     newID.String(),
				"description":      description,
				"originalFilename": fileHeader.Filename,
				"byteSize":         cr.n,
				"isSensitive":      isSensitive,
			},
		}); err != nil {
			_ = os.Remove(storePath)
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			_ = os.Remove(storePath)
			refuseInternal(w, err)
			return
		}

		out := attachmentRow{
			ID: newID, PropertyID: propertyID,
			Description: description, OriginalFilename: fileHeader.Filename,
			ContentType: contentType, ByteSize: cr.n,
			IsSensitive: isSensitive, ExtractState: "pending",
			CreatedAt: formatTime(createdAt), CreatedBy: c.Account.Username,
			UpdatedAt: formatTime(createdAt), UpdatedBy: c.Account.Username,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(attachmentResponse(out))
	})
}

// parentDir returns the directory portion of a path, used to ensure the
// store's sharded layout exists before opening a file inside it.
func parentDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "."
	}
	return p[:i]
}

// handleUpdateAttachment implements PATCH /attachments/{id}.
func (s *Server) handleUpdateAttachment(attachmentID uuid.UUID) http.Handler {
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
			Description *string `json:"description,omitempty"`
			IsSensitive *bool   `json:"isSensitive,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidJSON))
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
			if errors.Is(err, properties.ErrNotFound) {
				WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgAttachmentNotFound))
				return
			}
			refuseInternal(w, err)
			return
		}
		if archived, err := isPropertyArchived(r.Context(), tx, a.PropertyID); err != nil {
			refuseInternal(w, err)
			return
		} else if archived {
			WriteError(w, NewAPIError(http.StatusConflict, CodeArchived, MsgAttachmentArchivedRefused))
			return
		}
		if body.IsSensitive != nil && !*body.IsSensitive && a.IsSensitive {
			WriteError(w, NewAPIError(http.StatusConflict, CodeConflict, MsgAttachmentDemotionRefused))
			return
		}

		newDescription := a.Description
		if body.Description != nil {
			d := strings.TrimSpace(*body.Description)
			if n := len([]rune(d)); n < 2 || n > 200 {
				WriteError(w, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
					WithField("description", MsgAttachmentDescriptionLength))
				return
			}
			newDescription = d
		}
		newSensitive := a.IsSensitive
		if body.IsSensitive != nil {
			newSensitive = *body.IsSensitive
		}

		var updatedAt time.Time
		if err := tx.QueryRow(r.Context(), `
			UPDATE attachment
			SET description = $2, description_normalized = $5,
			    is_sensitive = $3, updated_at = now(), updated_by = $4
			WHERE id = $1
			RETURNING updated_at
		`, attachmentID, newDescription, newSensitive, c.Account.ID,
			identity.Canonical(newDescription)).Scan(&updatedAt); err != nil {
			refuseInternal(w, err)
			return
		}

		// On promotion, seal any existing text row inside the same
		// transaction so the search index never sees the readable body
		// for an instant. The row's body_normalized is set to NULL in the
		// same statement (research.md D-008).
		if body.IsSensitive != nil && *body.IsSensitive && !a.IsSensitive {
			var existingBody *string
			if err := tx.QueryRow(r.Context(),
				`SELECT body FROM attachment_text WHERE attachment_id = $1`, attachmentID).Scan(&existingBody); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				refuseInternal(w, err)
				return
			}
			if existingBody != nil && *existingBody != "" {
				sealed, err := s.envelope.Seal([]byte(*existingBody))
				if err != nil {
					refuseInternal(w, err)
					return
				}
				if _, err := tx.Exec(r.Context(), `
					UPDATE attachment_text
					SET body = NULL, body_normalized = NULL,
					    cipher_body = $2, cipher_nonce = $3,
					    wrapped_dek = $4, wrap_nonce = $5,
					    truncated = false,
					    corrected_at = NULL, corrected_by = NULL
					WHERE attachment_id = $1
				`, attachmentID, sealed.Ciphertext, sealed.CipherNonce, sealed.WrappedDEK, sealed.WrapNonce); err != nil {
					refuseInternal(w, err)
					return
				}
			}
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		before := map[string]any{}
		after := map[string]any{}
		if body.Description != nil && *body.Description != a.Description {
			before["description"] = a.Description
			after["description"] = newDescription
		}
		if body.IsSensitive != nil && *body.IsSensitive != a.IsSensitive {
			before["isSensitive"] = a.IsSensitive
			after["isSensitive"] = newSensitive
		}
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         actionForUpdate(body, a),
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       a.PropertyID.String(),
			SourceIP:       ClientIP(r),
			Before:         before,
			After:          after,
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}

		a.Description = newDescription
		a.IsSensitive = newSensitive
		a.UpdatedAt = formatTime(updatedAt)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(attachmentResponse(a))
	})
}

// handleDeleteAttachment implements DELETE /attachments/{id}.
func (s *Server) handleDeleteAttachment(attachmentID uuid.UUID) http.Handler {
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
			if errors.Is(err, properties.ErrNotFound) {
				WriteError(w, NewAPIError(http.StatusNotFound, CodeNotFound, MsgAttachmentNotFound))
				return
			}
			refuseInternal(w, err)
			return
		}
		if archived, err := isPropertyArchived(r.Context(), tx, a.PropertyID); err != nil {
			refuseInternal(w, err)
			return
		} else if archived {
			WriteError(w, NewAPIError(http.StatusConflict, CodeArchived, MsgAttachmentArchivedRefused))
			return
		}

		if _, err := tx.Exec(r.Context(), `DELETE FROM attachment WHERE id = $1`, attachmentID); err != nil {
			refuseInternal(w, err)
			return
		}

		actorID := c.Account.ID
		actorRole := c.Account.Role
		if err := audit.Write(r.Context(), tx, audit.Entry{
			Action:         audit.AttachmentRemoved,
			ActorAccountID: &actorID,
			ActorUsername:  c.Account.Username,
			ActorRole:      &actorRole,
			EntityType:     audit.EntityProperty,
			EntityID:       a.PropertyID.String(),
			SourceIP:       ClientIP(r),
			Before: map[string]any{
				"attachmentId":     attachmentID.String(),
				"description":      a.Description,
				"originalFilename": a.OriginalFilename,
				"isSensitive":      a.IsSensitive,
			},
		}); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			refuseInternal(w, err)
			return
		}
		if err := s.blobstore.Delete(attachmentID); err != nil {
			slog.Warn("blobstore delete after attachment row delete", "err", err, "attachment_id", attachmentID)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// requirePropertyExists is the same check, returning ErrNotFound when
// missing. Used by the list handler which does not care about archived.
func requirePropertyExists(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var archived bool
	row := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM property WHERE id = $1`, id)
	if err := row.Scan(&archived); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return properties.ErrNotFound
		}
		return err
	}
	return nil
}

// isPropertyArchived returns true when the property exists and is archived.
func isPropertyArchived(ctx context.Context, tx pgx.Tx, id uuid.UUID) (bool, error) {
	var archived bool
	row := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM property WHERE id = $1`, id)
	if err := row.Scan(&archived); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, properties.ErrNotFound
		}
		return false, err
	}
	return archived, nil
}

// loadAttachmentRow reads one row by id.
func loadAttachmentRow(ctx context.Context, tx pgx.Tx, id uuid.UUID) (attachmentRow, error) {
	var a attachmentRow
	var extractErr *string
	var createdAt, updatedAt time.Time
	row := tx.QueryRow(ctx, `
		SELECT a.id, a.property_id, a.description, a.original_filename,
		       a.content_type, a.byte_size, a.is_sensitive, a.extract_state,
		       a.extract_error, a.created_at, cu.username, a.updated_at, uu.username
		FROM attachment a
		JOIN account cu ON cu.id = a.created_by
		JOIN account uu ON uu.id = a.updated_by
		WHERE a.id = $1
	`, id)
	if err := row.Scan(&a.ID, &a.PropertyID, &a.Description, &a.OriginalFilename,
		&a.ContentType, &a.ByteSize, &a.IsSensitive, &a.ExtractState,
		&extractErr, &createdAt, &a.CreatedBy, &updatedAt, &a.UpdatedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return a, properties.ErrNotFound
		}
		return a, err
	}
	a.ExtractError = extractErr
	a.CreatedAt = formatTime(createdAt)
	a.UpdatedAt = formatTime(updatedAt)
	return a, nil
}

// actionForUpdate chooses the right audit action for a PATCH.
func actionForUpdate(body struct {
	Description *string `json:"description,omitempty"`
	IsSensitive *bool   `json:"isSensitive,omitempty"`
}, a attachmentRow) audit.Action {
	if body.IsSensitive != nil && *body.IsSensitive && !a.IsSensitive {
		return audit.AttachmentSensitivityRaised
	}
	return audit.AttachmentDescribed
}

// isAcceptedKind gates which sniffer results reach the store. Everything
// else is refused with unsupported_type before the row is written
// (FR-004, FR-004a).
func isAcceptedKind(k extract.Kind) bool {
	switch k {
	case extract.KindPDF, extract.KindOOXML, extract.KindImage:
		return true
	default:
		return false
	}
}

// getMultipartFile returns the first "file" part of the multipart body.
func getMultipartFile(r *http.Request) (*multipart.FileHeader, error) {
	if r.MultipartForm == nil {
		return nil, errors.New("no multipart form parsed")
	}
	for _, hdrs := range r.MultipartForm.File {
		for _, h := range hdrs {
			if h != nil {
				return h, nil
			}
		}
	}
	return nil, errors.New("no file part")
}

// countingReader counts bytes read so the handler can enforce the cap after
// the encrypt finishes (the cap cannot be known to the chunked encryptor
// up front — plaintext length is supplied, not enforced).
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// compile-time guard that pkg references stay.
var _ = pmscrypto.ChunkSize
