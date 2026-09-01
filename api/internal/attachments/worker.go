// attachments/worker.go — in-process extraction job runner.
//
// The state of an extraction lives on the attachment row (research.md D-006).
// This worker polls for due rows, claims one with a conditional UPDATE so
// two workers cannot race, and runs the chosen extractor. A row stays due
// only while extract_state = 'pending' AND extract_next_at <= now(). The
// trigger migration 0016 enforces the due-only-when-pending invariant in
// the schema.
//
// Backoff: attempts 1, 2, 3 at 30s, 2m, 10m. After three attempts the row
// becomes 'failed' and stops being due.

package attachments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	pmscrypto "pms/internal/crypto"
	"pms/internal/extract"
	"pms/internal/identity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// retryDelays is the schedule of growing waits between attempts.
var retryDelays = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

// Worker is the extraction job runner. It owns no long-lived state; one is
// started at server boot and stopped on shutdown.
type Worker struct {
	pool     *pgxpool.Pool
	envelope *pmscrypto.Envelope
	caps     extract.Capabilities
	maxText  int64
	tick     time.Duration
}

// NewWorker builds the runner. maxText is the configured cap on stored
// extracted text (FR-021).
func NewWorker(pool *pgxpool.Pool, env *pmscrypto.Envelope, caps extract.Capabilities, maxText int64) *Worker {
	return &Worker{
		pool:     pool,
		envelope: env,
		caps:     caps,
		maxText:  maxText,
		tick:     2 * time.Second,
	}
}

// Run blocks until ctx is cancelled. Errors are logged and the loop
// continues; a transient database hiccup never kills the worker.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(w.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for {
				more, err := w.processOne(ctx)
				if err != nil {
					slog.Warn("worker process", "err", err)
					break
				}
				if !more {
					break
				}
			}
		}
	}
}

// processOne claims and processes one due row. Returns (true, nil) if a
// row was claimed and processed (or marked failed/not_eligible), (false,
// nil) when there is nothing to do, or (false, err) on a system error.
func (w *Worker) processOne(ctx context.Context) (bool, error) {
	id, attempts, kindStr, isSensitive, err := w.claimNext(ctx)
	if err != nil {
		return false, err
	}
	if id == uuid.Nil {
		return false, nil
	}

	// kindStr is the stored content type, not a Kind token. Mapping is
	// required; casting sends everything to the unsupported-type branch.
	kind := extract.KindForContentType(kindStr)
	decision := extract.Route(ctx, kind, w.caps)

	if decision.State == "not_eligible" {
		if err := w.markNotEligible(ctx, id, decision.Reason); err != nil {
			return true, err
		}
		return true, nil
	}

	// Stream the plaintext body through the chosen extractor. The body is
	// decrypted in memory and piped over stdin to the subprocess, so no
	// plaintext ever touches disk (research.md D-004).
	text, extractErr := w.runExtractor(ctx, id, decision)

	switch {
	case errors.Is(extractErr, extract.ErrTooManyPages):
		// The document is beyond the processing bound. It is stored and stays
		// readable; only its text is out of reach (FR-019 `too_large`).
		if err := w.markState(ctx, id, "too_large", "pdf-too-many-pages"); err != nil {
			return true, err
		}
		return true, nil
	case extractErr != nil:
		// Bounded retry: schedule the next attempt or stop.
		next := attempts + 1
		if next >= int64(len(retryDelays)) {
			if err := w.markFailed(ctx, id, classifyError(extractErr)); err != nil {
				return true, err
			}
		} else {
			when := time.Now().Add(retryDelays[next])
			if err := w.scheduleRetry(ctx, id, when, classifyError(extractErr)); err != nil {
				return true, err
			}
		}
		return true, nil
	case text == "":
		if err := w.markEmpty(ctx, id, isSensitive); err != nil {
			return true, err
		}
		return true, nil
	default:
		// Cap and seal as appropriate.
		text, truncated := capText(text, w.maxText)
		if err := w.storeText(ctx, id, isSensitive, text, truncated); err != nil {
			return true, err
		}
		if err := w.markDone(ctx, id); err != nil {
			return true, err
		}
		return true, nil
	}
}

// claimNext picks the next due row, marks it claimed, and returns its
// identity, current attempt count, content kind, and sensitivity. The
// claim uses a conditional UPDATE so two workers cannot both win.
func (w *Worker) claimNext(ctx context.Context) (uuid.UUID, int64, string, bool, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, 0, "", false, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var id uuid.UUID
	var attempts int64
	var contentType string
	var isSensitive bool
	err = tx.QueryRow(ctx, `
		SELECT id, extract_attempts, content_type, is_sensitive
		FROM attachment
		WHERE extract_state = 'pending'
		  AND (extract_next_at IS NULL OR extract_next_at <= now())
		ORDER BY extract_next_at NULLS FIRST, created_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(&id, &attempts, &contentType, &isSensitive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, 0, "", false, nil
		}
		return uuid.Nil, 0, "", false, fmt.Errorf("select due: %w", err)
	}
	// No state change — the row is still pending; we just hold the lock
	// so a second worker skips it via SKIP LOCKED.
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, 0, "", false, fmt.Errorf("commit claim: %w", err)
	}
	return id, attempts, contentType, isSensitive, nil
}

// runExtractor streams the body through the chosen extractor.
func (w *Worker) runExtractor(ctx context.Context, id uuid.UUID, decision extract.Decision) (string, error) {
	// Open the store and stream-decrypt into the extractor.
	_, rc, err := openAttachmentBody(ctx, w.pool, w.envelope, id)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return decision.Run(ctx, rc)
}

// openBody opens the stored ciphertext, reads its header, and returns a
// reader positioned past it so the extractor can pipe plaintext in.
func (w *Worker) openBody(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	// The store, header, and decryption all live in their own packages;
	// the worker composes them.
	_, rc, err := openAttachmentBody(ctx, w.pool, w.envelope, id)
	if err != nil {
		return nil, err
	}
	return rc, nil
}

func (w *Worker) markNotEligible(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := w.pool.Exec(ctx, `
		UPDATE attachment
		SET extract_state = 'not_eligible', extract_error = $2, extract_next_at = NULL
		WHERE id = $1
	`, id, reason)
	return err
}

func (w *Worker) markFailed(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := w.pool.Exec(ctx, `
		UPDATE attachment
		SET extract_state = 'failed', extract_error = $2,
		    extract_attempts = extract_attempts + 1,
		    extract_next_at = NULL
		WHERE id = $1
	`, id, reason)
	return err
}

func (w *Worker) scheduleRetry(ctx context.Context, id uuid.UUID, when time.Time, reason string) error {
	_, err := w.pool.Exec(ctx, `
		UPDATE attachment
		SET extract_attempts = extract_attempts + 1,
		    extract_next_at = $2,
		    extract_error = $3
		WHERE id = $1
	`, id, when, reason)
	return err
}

func (w *Worker) markEmpty(ctx context.Context, id uuid.UUID, isSensitive bool) error {
	// Empty: nothing readable. A row is still written so the API has a body to
	// return — but it MUST take the shape the attachment's sensitivity demands.
	// Writing '' into body_normalized for a sensitive attachment would put that
	// row into the partial search index (WHERE body_normalized IS NOT NULL),
	// which is the structural exclusion this feature relies on. An empty string
	// cannot match a search term, so nothing would leak, but the invariant that
	// makes the exclusion trustworthy would be broken.
	if err := w.storeText(ctx, id, isSensitive, "", false); err != nil {
		return err
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE attachment SET extract_state = 'empty', extract_error = NULL, extract_next_at = NULL
		WHERE id = $1
	`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Worker) storeText(ctx context.Context, id uuid.UUID, isSensitive bool, text string, truncated bool) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var (
		bodyCol, normCol                       any
		cipherCol, cipherNCol, dekCol, dekNCol any
	)
	if isSensitive {
		sealed, err := w.envelope.Seal([]byte(text))
		if err != nil {
			return err
		}
		cipherCol = sealed.Ciphertext
		cipherNCol = sealed.CipherNonce
		dekCol = sealed.WrappedDEK
		dekNCol = sealed.WrapNonce
	} else {
		bodyCol = text
		n := identity.Canonical(text)
		normCol = n
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO attachment_text (
			attachment_id, body, body_normalized,
			cipher_body, cipher_nonce, wrapped_dek, wrap_nonce,
			truncated, extracted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (attachment_id) DO UPDATE SET
			body = EXCLUDED.body,
			body_normalized = EXCLUDED.body_normalized,
			cipher_body = EXCLUDED.cipher_body,
			cipher_nonce = EXCLUDED.cipher_nonce,
			wrapped_dek = EXCLUDED.wrapped_dek,
			wrap_nonce = EXCLUDED.wrap_nonce,
			truncated = EXCLUDED.truncated,
			extracted_at = now(),
			corrected_at = NULL, corrected_by = NULL
	`, id, bodyCol, normCol, cipherCol, cipherNCol, dekCol, dekNCol, truncated); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// markState records a terminal extraction state that is not a failure and is
// not retried. extract_error carries a reason code, never document content.
func (w *Worker) markState(ctx context.Context, id uuid.UUID, state, reason string) error {
	_, err := w.pool.Exec(ctx, `
		UPDATE attachment
		   SET extract_state = $2, extract_error = $3, extract_next_at = NULL
		 WHERE id = $1
	`, id, state, reason)
	return err
}

func (w *Worker) markDone(ctx context.Context, id uuid.UUID) error {
	_, err := w.pool.Exec(ctx, `
		UPDATE attachment SET extract_state = 'done', extract_error = NULL, extract_next_at = NULL
		WHERE id = $1
	`, id)
	return err
}

// classifyError turns a tool-level error into a short reason code suitable
// for storing in extract_error (which must be a code, never content).
func classifyError(err error) string {
	switch {
	case errors.Is(err, extract.ErrNoTesseract):
		return "tesseract-missing"
	case errors.Is(err, extract.ErrNoArabicPack):
		return "tesseract-no-arabic"
	case errors.Is(err, extract.ErrNoPoppler):
		return "poppler-missing"
	default:
		return "tool-error"
	}
}

// capText caps text at maxBytes on a whole-character (rune) boundary and
// reports whether the cap was applied (FR-021, FR-021b).
func capText(s string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 {
		return s, false
	}
	if int64(len(s)) <= maxBytes {
		return s, false
	}
	// Walk runes until we cross the boundary, then back up one.
	cut := 0
	for i, r := range s {
		if i+len(string(r)) > int(maxBytes) {
			break
		}
		cut = i + len(string(r))
	}
	return s[:cut], true
}

// openAttachmentBody is the helper that ties blobstore, crypto, and the
// worker together. It is package-private so the worker's other functions
// do not have to know about file descriptors.
func openAttachmentBody(ctx context.Context, pool *pgxpool.Pool, env *pmscrypto.Envelope, id uuid.UUID) (*pmscrypto.Header, io.ReadCloser, error) {
	return attachmentBodyOpener(ctx, pool, env, id)
}
