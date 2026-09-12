// Package attachments is the persistence layer for the
// 003-property-attachments-ocr feature. It composes the lower-level packages
// (crypto, blobstore, extract) without owning any of their state, so the
// rule "no plaintext byte on disk" is enforced by callers (the handlers and
// the worker) rather than by accident of how data flows through here.
//
// Three layers live in this package:
//   - Store: SQL methods that read and write attachment rows and their text.
//   - Worker: an in-process job runner that polls for due rows, claims them,
//     runs the chosen extractor, and writes back the result.
//   - SearchStore: the document search query (US5).
package attachments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the persistence entry point. It holds the pool; methods take a
// pgx.Tx so the caller controls transaction boundaries. The handlers that
// write a row plus an audit row in one transaction stay in charge of both.
type Store struct {
	Pool interface {
		Begin(context.Context) (pgx.Tx, error)
	}
}

// ErrArchived means the attachment's parent property is archived and the
// operation is not allowed (FR-006).
var ErrArchived = errors.New("property is archived")

// ErrNotFound means no attachment has that id.
var ErrNotFound = errors.New("attachment not found")

// ErrCorrected means re-extraction was attempted on an attachment whose text
// was corrected by hand — the operation would discard the correction and is
// refused (FR-020c).
var ErrCorrected = errors.New("attachment text has been corrected; clear the correction first")

// ErrAlreadyRunning means a re-extraction is requested for an attachment
// whose extraction is already in progress (FR-020d).
var ErrAlreadyRunning = errors.New("attachment extraction already running")

// ErrInvalidState means re-extraction was requested from a state that does
// not permit it (only `failed` and `empty` do; FR-020b).
var ErrInvalidState = errors.New("attachment state does not permit re-extraction")

// Attachment is the in-memory representation of one row.
type Attachment struct {
	ID               uuid.UUID
	PropertyID       uuid.UUID
	Description      string
	OriginalFilename string
	ContentType      string
	ByteSize         int64
	WrappedDEK       []byte
	WrapNonce        []byte
	NoncePrefix      []byte
	ChunkSize        int32
	IsSensitive      bool
	ExtractState     string
	ExtractAttempts  int
	ExtractNextAt    *string // nullable; rendered as RFC3339 in API responses
	ExtractError     *string
	CreatedAt        string
	CreatedBy        string
	UpdatedAt        string
	UpdatedBy        string
}

// propertyArchivedStatus is the small set of facts a writer needs about a
// property before it can attach an attachment: whether the property exists
// and whether it is archived. Used by CreateAttachment and UpdateAttachment.
type propertyArchivedStatus struct {
	exists   bool
	archived bool
}

func resolvePropertyArchived(ctx context.Context, tx pgx.Tx, propertyID uuid.UUID) (propertyArchivedStatus, error) {
	var status propertyArchivedStatus
	row := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM property WHERE id = $1`, propertyID)
	if err := row.Scan(&status.archived); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return status, nil
		}
		return status, fmt.Errorf("resolve property: %w", err)
	}
	status.exists = true
	return status, nil
}

// newPoolTx is a small helper that opens a transaction on the store's pool.
// It exists so call sites do not have to import pgxpool directly when they
// only need a one-shot write transaction (e.g. the read-audit commit in
// the content handler, which is in httpx, not here).
func newPoolTx(pool *pgxpool.Pool) pgx.Tx {
	tx, err := pool.Begin(context.Background())
	if err != nil {
		slog.Error("begin tx", "err", err)
		return nil
	}
	return tx
}

// itoa is a tiny helper kept private to this package so we don't pull in
// strconv just for parameter index placeholders. The same trick is used
// elsewhere in the project (properties/store.go).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
