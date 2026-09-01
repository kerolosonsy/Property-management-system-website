// stream_test.go — chunked AES-256-GCM streaming. These cases target the
// places where a mistake is silent and invisible on screen (Constitution V,
// tasks.md T012).
//
// Coverage:
//   - happy-path round trip across multiple chunks
//   - tampered ciphertext body for one chunk fails closed
//   - two chunks swapped (final-chunk flag and chunk index are in the AAD)
//   - truncation: a file cut short fails because the last chunk does not
//     claim to be final
//   - a wrong KEK unwrapping the data key fails closed

package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"testing"
)

func newEnvelope(t *testing.T, seed byte) *Envelope {
	t.Helper()
	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = seed
	}
	env, err := New(kek)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	return env
}

func TestStreamRoundTrip(t *testing.T) {
	env := newEnvelope(t, 0x42)
	plaintext := bytes.Repeat([]byte("pms-"), ChunkSize/4+7) // ~3 chunks
	plaintext = append(plaintext, bytes.Repeat([]byte{0xAB}, ChunkSize+1)...)
	// Pad to a deterministic non-multiple-of-chunk size to exercise final-chunk handling.
	for len(plaintext)%ChunkSize == 0 {
		plaintext = append(plaintext, 'X')
	}

	var sealed bytes.Buffer
	hdr, err := env.Stream(bytes.NewReader(plaintext), &sealed, int64(len(plaintext)))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if hdr.PlaintextLength != uint64(len(plaintext)) {
		t.Fatalf("plaintext length in header: got %d want %d", hdr.PlaintextLength, len(plaintext))
	}

	// Round trip. OpenStream expects the reader positioned past the header;
	// in production the storage layer reads the header (and validates it
	// against the database) before decrypting.
	_, hdrLen, err := ReadHeader(bytes.NewReader(sealed.Bytes()))
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	var out bytes.Buffer
	if err := env.OpenStream(hdr, bytes.NewReader(sealed.Bytes()[hdrLen:]), &out); err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if !bytes.Equal(out.Bytes(), plaintext) {
		t.Fatalf("plaintext mismatch (got %d bytes, want %d)", out.Len(), len(plaintext))
	}
}

func TestStreamRejectsTamperedChunk(t *testing.T) {
	env := newEnvelope(t, 0x11)
	plaintext := bytes.Repeat([]byte{0xCD}, ChunkSize*2+123)
	var sealed bytes.Buffer
	hdr, err := env.Stream(bytes.NewReader(plaintext), &sealed, int64(len(plaintext)))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	// Flip one byte in the second chunk's ciphertext body. The GCM tag
	// verification MUST fail and produce no plaintext.
	buf := sealed.Bytes()
	// Skip past the header — its exact size is computed from the wrapped
	// key length, which varies. We need to position past it; the simplest
	// way is to read the header first and use its offsets.
	_, hdrLen, err := ReadHeader(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	target := hdrLen + ChunkSize + cryptoOverhead + 5
	if target >= int64(len(buf)) {
		t.Fatalf("test buffer too small to tamper (len=%d, target=%d)", len(buf), target)
	}
	buf[target] ^= 0xFF

	var out bytes.Buffer
	err = env.OpenStream(hdr, bytes.NewReader(buf), &out)
	if err == nil {
		t.Fatalf("tamper accepted; %d bytes of plaintext emitted", out.Len())
	}
	if out.Len() != 0 {
		t.Fatalf("partial plaintext emitted: %d bytes", out.Len())
	}
}

// TestStreamRejectsSwappedChunks swaps two chunks and expects OpenStream to
// fail because the chunk index is bound into each chunk's AAD — without that
// binding, an attacker who can write to the store could swap chunks and
// every chunk would still verify on its own (research.md D-001).
func TestStreamRejectsSwappedChunks(t *testing.T) {
	env := newEnvelope(t, 0x22)
	plaintext := bytes.Repeat([]byte{0xEF}, ChunkSize*4)
	var sealed bytes.Buffer
	hdr, err := env.Stream(bytes.NewReader(plaintext), &sealed, int64(len(plaintext)))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	buf := sealed.Bytes()
	_, hdrLen, err := ReadHeader(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	// Swap chunk 0 and chunk 1. Their sizes may differ only if the last
	// chunk is shorter, which it isn't here.
	frameSize := int64(ChunkSize + cryptoOverhead)
	a := hdrLen
	b := hdrLen + frameSize
	frameA := append([]byte{}, buf[a:b]...)
	copy(buf[a:b], buf[b:b+frameSize])
	copy(buf[b:b+frameSize], frameA)

	var out bytes.Buffer
	err = env.OpenStream(hdr, bytes.NewReader(buf), &out)
	if err == nil {
		t.Fatalf("swap accepted; %d bytes of plaintext emitted", out.Len())
	}
	if out.Len() != 0 {
		t.Fatalf("partial plaintext emitted: %d bytes", out.Len())
	}
}

func TestStreamRejectsTruncation(t *testing.T) {
	env := newEnvelope(t, 0x33)
	plaintext := bytes.Repeat([]byte{0x77}, ChunkSize*3+200)
	var sealed bytes.Buffer
	hdr, err := env.Stream(bytes.NewReader(plaintext), &sealed, int64(len(plaintext)))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	// Drop the last chunk.
	cut := int64(sealed.Len()) - (int64(ChunkSize) + cryptoOverhead)
	if cut <= 0 {
		t.Fatalf("test buffer unexpectedly small")
	}
	truncated := sealed.Bytes()[:cut]

	var out bytes.Buffer
	err = env.OpenStream(hdr, bytes.NewReader(truncated), &out)
	if err == nil {
		t.Fatalf("truncation accepted; %d bytes of plaintext emitted", out.Len())
	}
}

func TestStreamRejectsWrongKEK(t *testing.T) {
	writeEnv := newEnvelope(t, 0x44)
	plaintext := bytes.Repeat([]byte{0x88}, ChunkSize*2+1)
	var sealed bytes.Buffer
	hdr, err := writeEnv.Stream(bytes.NewReader(plaintext), &sealed, int64(len(plaintext)))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	readEnv := newEnvelope(t, 0x55)
	var out bytes.Buffer
	err = readEnv.OpenStream(hdr, bytes.NewReader(sealed.Bytes()), &out)
	if err == nil {
		t.Fatalf("wrong KEK accepted; %d bytes of plaintext emitted", out.Len())
	}
}

// cryptoOverhead is the per-chunk bytes added by GCM (16-byte tag) plus the
// 12-byte nonce stored alongside. We keep it here, local to the test, to
// avoid coupling the public API.
const cryptoOverhead = 16 + 12

// Sanity: the construction refuses nil sources, etc. This is a guard, not a
// feature — most errors come out of the underlying I/O and AEAD primitives.
func TestStreamRejectsNilHeader(t *testing.T) {
	env := newEnvelope(t, 0x66)
	if err := env.OpenStream(nil, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("nil header accepted")
	}
}

// TestStreamRejectsBadMagic protects against a caller handing us a
// non-encrypted file and us silently writing garbage to disk.
func TestStreamRejectsBadMagic(t *testing.T) {
	if _, _, err := ReadHeader(bytes.NewReader([]byte{0x00, 0x00, 'h', 'i'})); err == nil {
		t.Fatal("bad magic accepted")
	}
}

// TestStreamZeroByteFile covers the degenerate case of a zero-byte upload.
// The header still goes to disk, no chunks are written, and OpenStream is a
// no-op. Principle VII forbids storing nothing encrypted, so the row must
// still exist with byte_size = 0; the FR-005 size check is the handler's
// responsibility, not this code's.
func TestStreamZeroByteFile(t *testing.T) {
	env := newEnvelope(t, 0x77)
	var sealed bytes.Buffer
	hdr, err := env.Stream(bytes.NewReader(nil), &sealed, 0)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var out bytes.Buffer
	if err := env.OpenStream(hdr, bytes.NewReader(sealed.Bytes()), &out); err != nil {
		t.Fatalf("open: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected 0 bytes, got %d", out.Len())
	}
}

// Compile-time guards that the helpers used in the assertions above exist.
var _ = rand.Reader
var _ = io.EOF
var _ = errors.New

// TestStreamRejectsUnderDeclaredLength guards the defect that made every upload
// store a header and no body: Stream is length-driven, and a caller passing a
// length shorter than the source (0, most damagingly) would encrypt nothing and
// report success. The source must be exhausted or the call must fail.
func TestStreamRejectsUnderDeclaredLength(t *testing.T) {
	env := newEnvelope(t, 0x44)
	plaintext := bytes.Repeat([]byte{0xAB}, ChunkSize*2)

	var sealed bytes.Buffer
	if _, err := env.Stream(bytes.NewReader(plaintext), &sealed, 0); err == nil {
		t.Fatal("Stream accepted a declared length of 0 for a non-empty source; it must refuse")
	}

	sealed.Reset()
	if _, err := env.Stream(bytes.NewReader(plaintext), &sealed, int64(len(plaintext)/2)); err == nil {
		t.Fatal("Stream accepted a short declared length; it must refuse")
	}

	sealed.Reset()
	if _, err := env.Stream(bytes.NewReader(plaintext), &sealed, int64(len(plaintext))); err != nil {
		t.Fatalf("Stream rejected the correct length: %v", err)
	}
}
