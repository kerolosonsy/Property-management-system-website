// Package crypto — chunked AES-256-GCM streaming for attachment file bodies
// (feature 003-property-attachments-ocr, research.md D-001, D-002).
//
// This is a sibling of envelope.go, which implements the single-shot
// Seal/Open used for sensitive values and for a sensitive attachment's
// extracted text. Bodies are large and must stream; values are small and
// must not. Both schemes share the same master key, the same data-key-
// wrapping under that master key, and the same per-chunk GCM tag.
//
// The construction:
//   - one fresh 256-bit data key per file, wrapped under the KEK;
//   - a fixed plaintext chunk size (default 64 KiB);
//   - a per-file random 8-byte nonce prefix; each chunk's nonce is
//     prefix || counter (4 bytes big-endian);
//   - each chunk's additional authenticated data is the chunk index and a
//     final-chunk flag — this is what makes reordering, splicing, and
//     truncation detectable rather than producing plausible-looking
//     plaintext (research.md D-001);
//   - the file header stores the wrapped key, the prefix, the chunk size,
//     and the plaintext length so a truncated file is caught before any
//     byte is served.
//
// No plaintext byte is ever written to disk by this code. The encrypt
// direction encrypts and streams to an io.Writer; the decrypt direction
// streams decrypted chunks to an io.Writer as each tag verifies.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ChunkSize is the fixed plaintext chunk size used by Stream. 64 KiB is the
// value research.md D-001 settled on: large enough to amortise the per-chunk
// overhead, small enough that the largest file bounded by FR-005 (50 MB)
// never holds more than a single chunk's worth in memory per concurrent
// stream.
const ChunkSize = 64 * 1024

// HeaderMagic is the first two bytes of the file header. It is here so a
// caller can sanity-check that a stream they are about to read is one of
// ours, not a random binary blob.
const HeaderMagic uint16 = 0x5047 // "PG" for PMS-GCM

// Header is the file header written at the start of every encrypted
// attachment body. PlaintextLength is the original file size, recorded at
// seal time so a truncated file is caught before any byte is served, not
// only at the end (data-model.md "Encryption layout").
type Header struct {
	Magic           uint16
	WrappedDEK      []byte
	WrapNonce       []byte
	NoncePrefix     []byte
	ChunkSize       uint32
	PlaintextLength uint64
}

// headerLayout is the on-disk shape:
//
//	magic       u16
//	wrapped_len u32
//	wrapped     bytes
//	wrap_nonce  bytes (= 12)
//	prefix_len  u8 (= 8)
//	prefix      bytes (= 8)
//	chunk_size  u32
//	plain_len   u64
//
// The nonces are fixed-size, which is what the AEAD constructor tells us,
// so no length prefix is needed for those.
const (
	wrapNonceLen = 12
	prefixLen    = 8
	aadIndexLen  = 4
	aadFinalLen  = 1
	aadLen       = aadIndexLen + aadFinalLen
)

// Stream encrypts a plaintext stream from src into dst. dst receives the file
// header followed by per-chunk frames, each a 12-byte nonce and the GCM
// ciphertext for that chunk. The caller supplies an io.Writer; the typical
// caller is the blob store, which writes through an *os.File.
//
// A fresh DEK is generated for every call. The wrapped form is what is
// stored in the database; the raw DEK never leaves this call.
func (e *Envelope) Stream(src io.Reader, dst io.Writer, plaintextLen int64) (*Header, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("rand dek: %w", err)
	}
	defer zeroize(dek)

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("new data cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new data gcm: %w", err)
	}

	wrapNonce := make([]byte, wrapNonceLen)
	if _, err := io.ReadFull(rand.Reader, wrapNonce); err != nil {
		return nil, fmt.Errorf("rand wrap nonce: %w", err)
	}
	wrapped := e.gcm.Seal(nil, wrapNonce, dek, nil)
	prefix := make([]byte, prefixLen)
	if _, err := io.ReadFull(rand.Reader, prefix); err != nil {
		return nil, fmt.Errorf("rand prefix: %w", err)
	}

	hdr := &Header{
		Magic:           HeaderMagic,
		WrappedDEK:      wrapped,
		WrapNonce:       wrapNonce,
		NoncePrefix:     prefix,
		ChunkSize:       uint32(ChunkSize),
		PlaintextLength: uint64(plaintextLen),
	}
	if err := writeHeader(dst, hdr); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	plain := make([]byte, ChunkSize)
	var counter uint32 = 0
	remaining := plaintextLen
	for remaining > 0 {
		n, err := io.ReadFull(src, plain)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return nil, fmt.Errorf("read chunk: %w", err)
		}
		// ReadFull returns ErrUnexpectedEOF when the source runs short.
		// That is the uploader's last chunk; anything else is a real error.
		if n == 0 && remaining > 0 {
			return nil, fmt.Errorf("plaintext short: expected %d, got 0", remaining)
		}
		final := remaining == int64(n)
		nonce := buildChunkNonce(prefix, counter)
		aad := buildChunkAAD(counter, final)
		ct := aead.Seal(nil, nonce, plain[:n], aad)
		if _, err := dst.Write(ct); err != nil {
			return nil, fmt.Errorf("write chunk %d: %w", counter, err)
		}
		counter++
		remaining -= int64(n)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if final {
			break
		}
	}

	// The source MUST be exhausted. A declared length shorter than the actual
	// input would otherwise store a truncated file and report success — which
	// is how a zero length silently produced header-only files. Reading one
	// more byte turns that class of mistake into a hard error.
	var probe [1]byte
	if n, err := src.Read(probe[:]); n > 0 {
		return nil, fmt.Errorf("plaintext longer than the declared length of %d", plaintextLen)
	} else if err != nil && err != io.EOF {
		return nil, fmt.Errorf("verify source exhausted: %w", err)
	}

	return hdr, nil
}

// OpenStream decrypts a stream produced by Stream. It writes plaintext to
// dst as each chunk's GCM tag verifies. Any verification failure aborts
// without producing a partial file (FR-014). PlaintextLength is the value
// the caller read from the database; the header's plain_len is sanity-
// checked against it.
func (e *Envelope) OpenStream(hdr *Header, src io.Reader, dst io.Writer) error {
	if hdr == nil {
		return errors.New("header is nil")
	}
	if hdr.Magic != HeaderMagic {
		return errors.New("not a chunked stream")
	}
	if hdr.ChunkSize == 0 {
		return errors.New("chunk size is zero")
	}

	dek, err := e.gcm.Open(nil, hdr.WrapNonce, hdr.WrappedDEK, nil)
	if err != nil {
		return fmt.Errorf("unwrap dek: %w", err)
	}
	defer zeroize(dek)

	block, err := aes.NewCipher(dek)
	if err != nil {
		return fmt.Errorf("new data cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("new data gcm: %w", err)
	}

	plain := make([]byte, int(hdr.ChunkSize)+aead.Overhead())
	remaining := int64(hdr.PlaintextLength)
	var counter uint32 = 0
	for remaining > 0 {
		ct := plain[:int(hdr.ChunkSize)+aead.Overhead()]
		chunkLen := int64(len(ct))
		if remaining <= int64(hdr.ChunkSize) {
			// Last chunk may be short: trim what we expect to read.
			chunkLen = remaining + int64(aead.Overhead())
			ct = make([]byte, chunkLen)
		}
		if _, err := io.ReadFull(src, ct); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return fmt.Errorf("truncated stream at chunk %d", counter)
			}
			return fmt.Errorf("read chunk %d: %w", counter, err)
		}
		nonce := buildChunkNonce(hdr.NoncePrefix, counter)
		final := remaining <= int64(hdr.ChunkSize)
		aad := buildChunkAAD(counter, final)
		pt, err := aead.Open(nil, nonce, ct, aad)
		if err != nil {
			return fmt.Errorf("verify chunk %d: %w", counter, err)
		}
		if _, err := dst.Write(pt); err != nil {
			return fmt.Errorf("write plaintext chunk %d: %w", counter, err)
		}
		counter++
		remaining -= int64(len(pt))
	}
	if remaining < 0 {
		return errors.New("stream produced more plaintext than the header declared")
	}
	return nil
}

// buildChunkNonce constructs the per-chunk 12-byte nonce: 8-byte prefix ||
// 4-byte big-endian counter. GCM's nonce uniqueness under one key is what
// makes this construction safe (research.md D-001).
func buildChunkNonce(prefix []byte, counter uint32) []byte {
	out := make([]byte, wrapNonceLen)
	copy(out, prefix)
	binary.BigEndian.PutUint32(out[prefixLen:], counter)
	return out
}

// buildChunkAAD constructs each chunk's additional authenticated data:
// 4-byte big-endian chunk index, 1-byte final-chunk flag (1 = final). This
// is what makes reordering, splicing, and truncation detectable: every
// chunk still verifies on its own, but with a forged or replayed index it
// does not (research.md D-001).
func buildChunkAAD(counter uint32, final bool) []byte {
	out := make([]byte, aadLen)
	binary.BigEndian.PutUint32(out[:aadIndexLen], counter)
	if final {
		out[aadIndexLen] = 1
	}
	return out
}

// writeHeader writes the on-disk header. Lengths for wrapped_dek are stored
// as a u32 so a corrupt header fails fast rather than reading garbage.
func writeHeader(dst io.Writer, h *Header) error {
	if _, err := dst.Write([]byte{byte(h.Magic >> 8), byte(h.Magic)}); err != nil {
		return err
	}
	if err := binary.Write(dst, binary.BigEndian, uint32(len(h.WrappedDEK))); err != nil {
		return err
	}
	if _, err := dst.Write(h.WrappedDEK); err != nil {
		return err
	}
	if _, err := dst.Write(h.WrapNonce); err != nil {
		return err
	}
	if _, err := dst.Write([]byte{byte(len(h.NoncePrefix))}); err != nil {
		return err
	}
	if _, err := dst.Write(h.NoncePrefix); err != nil {
		return err
	}
	if err := binary.Write(dst, binary.BigEndian, h.ChunkSize); err != nil {
		return err
	}
	return binary.Write(dst, binary.BigEndian, h.PlaintextLength)
}

// ReadHeader parses a header from src. The returned Header owns its slices;
// callers MUST treat it as read-only.
func ReadHeader(src io.Reader) (*Header, int64, error) {
	var magic [2]byte
	if _, err := io.ReadFull(src, magic[:]); err != nil {
		return nil, 0, fmt.Errorf("read magic: %w", err)
	}
	if uint16(magic[0])<<8|uint16(magic[1]) != HeaderMagic {
		return nil, 0, errors.New("not a chunked stream")
	}
	var wrappedLen uint32
	if err := binary.Read(src, binary.BigEndian, &wrappedLen); err != nil {
		return nil, 0, fmt.Errorf("read wrapped len: %w", err)
	}
	hdr := &Header{Magic: HeaderMagic}
	hdr.WrappedDEK = make([]byte, wrappedLen)
	if _, err := io.ReadFull(src, hdr.WrappedDEK); err != nil {
		return nil, 0, fmt.Errorf("read wrapped key: %w", err)
	}
	hdr.WrapNonce = make([]byte, wrapNonceLen)
	if _, err := io.ReadFull(src, hdr.WrapNonce); err != nil {
		return nil, 0, fmt.Errorf("read wrap nonce: %w", err)
	}
	var prefixLenByte uint8
	if err := binary.Read(src, binary.BigEndian, &prefixLenByte); err != nil {
		return nil, 0, fmt.Errorf("read prefix len: %w", err)
	}
	hdr.NoncePrefix = make([]byte, prefixLenByte)
	if _, err := io.ReadFull(src, hdr.NoncePrefix); err != nil {
		return nil, 0, fmt.Errorf("read prefix: %w", err)
	}
	if err := binary.Read(src, binary.BigEndian, &hdr.ChunkSize); err != nil {
		return nil, 0, fmt.Errorf("read chunk size: %w", err)
	}
	if err := binary.Read(src, binary.BigEndian, &hdr.PlaintextLength); err != nil {
		return nil, 0, fmt.Errorf("read plaintext length: %w", err)
	}

	// Total header length on disk so callers can position the file pointer
	// past it for raw reads. The math here must match writeHeader byte for
	// byte — anything off means a partial header on disk and a hard failure.
	total := int64(2 + 4 + int64(wrappedLen) + int64(wrapNonceLen) + 1 + int64(prefixLenByte) + 4 + 8)
	return hdr, total, nil
}
