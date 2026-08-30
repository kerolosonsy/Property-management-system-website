package crypto

import (
	"bytes"
	"testing"
)

// freshKEK returns a 32-byte key that the caller owns.
func freshKEK(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	env, err := New(freshKEK(t))
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	cases := []string{"", "x", "12345678901234", "القيد العقاري ١٢٣٤"}
	for _, s := range cases {
		s := s
		t.Run(s, func(t *testing.T) {
			sealed, err := env.Seal([]byte(s))
			if err != nil {
				t.Fatalf("seal: %v", err)
			}
			got, err := env.Open(sealed)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if !bytes.Equal(got, []byte(s)) {
				t.Fatalf("round trip: got %q want %q", got, s)
			}
		})
	}
}

func TestOpenFailsOnTamperedCiphertext(t *testing.T) {
	env, err := New(freshKEK(t))
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	sealed, err := env.Seal([]byte("hello"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	// Flip a byte well inside the plaintext region of the ciphertext.
	tampered := *sealed
	tampered.Ciphertext = append([]byte(nil), sealed.Ciphertext...)
	tampered.Ciphertext[len(tampered.Ciphertext)-1] ^= 0x01

	if got, err := env.Open(&tampered); err == nil {
		t.Fatalf("open accepted tampered ciphertext: %q", got)
	} else if got != nil {
		t.Fatalf("open returned plaintext on tamper: %q", got)
	}
}

func TestOpenFailsOnTamperedWrappedKey(t *testing.T) {
	env, err := New(freshKEK(t))
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	sealed, err := env.Seal([]byte("hello"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	tampered := *sealed
	tampered.WrappedDEK = append([]byte(nil), sealed.WrappedDEK...)
	tampered.WrappedDEK[0] ^= 0x01

	if got, err := env.Open(&tampered); err == nil {
		t.Fatalf("open accepted tampered wrapped key: %q", got)
	} else if got != nil {
		t.Fatalf("open returned plaintext on tamper: %q", got)
	}
}

func TestOpenFailsOnWrongKEK(t *testing.T) {
	_, err := New(freshKEK(t))
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}

	env1, err := New(freshKEK(t))
	if err != nil {
		t.Fatalf("new envelope 1: %v", err)
	}
	env2, err := New(bytes.Repeat([]byte{0xFF}, 32))
	if err != nil {
		t.Fatalf("new envelope 2: %v", err)
	}

	sealed, err := env1.Seal([]byte("hello"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	if got, err := env2.Open(sealed); err == nil {
		t.Fatalf("open accepted ciphertext with wrong KEK: %q", got)
	} else if got != nil {
		t.Fatalf("open returned plaintext on wrong KEK: %q", got)
	}
}

func TestNewRefusesWrongSizeKey(t *testing.T) {
	cases := []int{0, 16, 24, 31, 33, 64}
	for _, n := range cases {
		n := n
		t.Run("", func(t *testing.T) {
			if _, err := New(make([]byte, n)); err == nil {
				t.Fatalf("New accepted %d-byte key", n)
			}
		})
	}
}
