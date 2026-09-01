// attachments/body_opener.go — the worker opens an attachment's body from
// the store and decrypts it to a reader positioned past the header.

package attachments

import (
	"context"
	"io"

	"pms/internal/blobstore"
	pmscrypto "pms/internal/crypto"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// attachmentBodyOpener reads the crypto material for one row and returns
// a reader positioned past the file header that streams plaintext.
func attachmentBodyOpener(ctx context.Context, pool *pgxpool.Pool, env *pmscrypto.Envelope, id uuid.UUID) (*pmscrypto.Header, io.ReadCloser, error) {
	var wrappedDEK, wrapNonce, noncePrefix []byte
	var chunkSize int32
	var byteSize int64
	err := pool.QueryRow(ctx, `
		SELECT wrapped_dek, wrap_nonce, nonce_prefix, chunk_size, byte_size
		FROM attachment WHERE id = $1
	`, id).Scan(&wrappedDEK, &wrapNonce, &noncePrefix, &chunkSize, &byteSize)
	if err != nil {
		return nil, nil, err
	}
	hdr := &pmscrypto.Header{
		Magic:           pmscrypto.HeaderMagic,
		WrappedDEK:      wrappedDEK,
		WrapNonce:       wrapNonce,
		NoncePrefix:     noncePrefix,
		ChunkSize:       uint32(chunkSize),
		PlaintextLength: uint64(byteSize),
	}
	store := currentStore()
	if store == nil {
		return hdr, nil, errStoreNotConfigured
	}
	rc, err := store.Open(id)
	if err != nil {
		return hdr, nil, err
	}

	// The stored file starts with its own header. OpenStream expects src to be
	// positioned at the first ciphertext byte, so it MUST be consumed here —
	// otherwise the header is decrypted as chunk 0, authentication fails, and
	// the reader yields nothing.
	fileHdr, _, err := pmscrypto.ReadHeader(rc)
	if err != nil {
		_ = rc.Close()
		return hdr, nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		defer rc.Close()
		// CloseWithError, not Close: a decryption failure must reach the
		// extractor as an error. Closing silently would present a truncated or
		// empty document as a successful read, and the attachment would be
		// recorded as having no text rather than as having failed.
		if err := env.OpenStream(fileHdr, rc, pw); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()
	return fileHdr, pr, nil
}

// errStoreNotConfigured fires when the worker is used without a configured
// store. main wires the store in before the worker runs.
var errStoreNotConfigured = blobstore.ErrNotFound

// storeRef is the active store. Set by main once at startup.
var storeRef *blobstore.Store

// SetStore records the store the worker should use.
func SetStore(s *blobstore.Store) { storeRef = s }

func currentStore() *blobstore.Store { return storeRef }
