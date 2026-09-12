// Package crypto implements AES-256-GCM envelope encryption for designated
// sensitive custom field values (Constitution VII, feature 002-properties-crud,
// research.md D-003).
//
// One fresh random 256-bit data key (DEK) is generated per value. The DEK is
// wrapped under the master key (KEK) with its own AES-256-GCM seal, and the
// wrapped form is what is stored. The KEK is supplied once at process start
// from PMS_KEK and never leaves this package.
//
// The GCM authentication tag is verified on every Open; a failure is a hard
// error and returns no plaintext.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// Sealed is what gets written to the database for one sensitive value:
// ciphertext and the per-value data key, wrapped under the KEK. Both nonces
// are stored alongside.
type Sealed struct {
	Ciphertext []byte
	CipherNonce []byte
	WrappedDEK  []byte
	WrapNonce   []byte
}

// Envelope is the per-process object that owns the KEK and exposes seal/open.
type Envelope struct {
	gcm cipher.AEAD // GCM under the KEK, used to wrap each DEK
}

// New builds an Envelope from a 32-byte KEK. The KEK is held in memory for the
// life of the process; it is never logged, returned in an error, or written
// to storage (Constitution VII).
func New(kek []byte) (*Envelope, error) {
	if len(kek) != 32 {
		return nil, fmt.Errorf("KEK must be exactly 32 bytes; got %d", len(kek))
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		// aes.NewCipher only fails on an unsupported key size, which we have
		// already guarded against.
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Envelope{gcm: gcm}, nil
}

// Seal encrypts plaintext with a fresh DEK and wraps that DEK under the KEK.
// The returned Sealed carries every byte needed to recover the value later.
func (e *Envelope) Seal(plaintext []byte) (*Sealed, error) {
	if plaintext == nil {
		return nil, errors.New("plaintext is nil")
	}

	// 1. Fresh data key.
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("rand dek: %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("new data cipher: %w", err)
	}
	dekGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new data gcm: %w", err)
	}

	// 2. Encrypt plaintext with the DEK.
	cipherNonce := make([]byte, dekGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, cipherNonce); err != nil {
		return nil, fmt.Errorf("rand cipher nonce: %w", err)
	}
	ciphertext := dekGCM.Seal(nil, cipherNonce, plaintext, nil)

	// 3. Wrap the DEK under the KEK.
	wrapNonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, wrapNonce); err != nil {
		return nil, fmt.Errorf("rand wrap nonce: %w", err)
	}
	wrappedDEK := e.gcm.Seal(nil, wrapNonce, dek, nil)

	return &Sealed{
		Ciphertext:  ciphertext,
		CipherNonce: cipherNonce,
		WrappedDEK:   wrappedDEK,
		WrapNonce:    wrapNonce,
	}, nil
}

// Open decrypts a Sealed value. The DEK is unwrapped under the KEK and the
// GCM tag on the value is verified; a tampered ciphertext, a tampered wrapped
// key, a wrong KEK, or a swapped nonce all fail closed with an error and no
// plaintext.
func (e *Envelope) Open(s *Sealed) ([]byte, error) {
	if s == nil {
		return nil, errors.New("sealed value is nil")
	}
	if s.WrappedDEK == nil || s.WrapNonce == nil ||
		s.Ciphertext == nil || s.CipherNonce == nil {
		return nil, errors.New("sealed value is incomplete")
	}

	// 1. Unwrap DEK. Verifying the GCM tag here is what catches a wrong KEK
	// or a tampered wrapped key: a forged tag will fail the open and the
	// function returns no bytes.
	dek, err := e.gcm.Open(nil, s.WrapNonce, s.WrappedDEK, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrap dek: %w", err)
	}
	defer zeroize(dek)

	if len(dek) != 32 {
		return nil, fmt.Errorf("unwrapped dek is %d bytes, expected 32", len(dek))
	}

	// 2. Decrypt with the DEK. Same tag verification closes the ciphertext
	// against tampering.
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("new data cipher: %w", err)
	}
	dekGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new data gcm: %w", err)
	}
	plaintext, err := dekGCM.Open(nil, s.CipherNonce, s.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("open ciphertext: %w", err)
	}
	return plaintext, nil
}

// zeroize overwrites a buffer so the data key does not outlive the call.
func zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
