// Package auth implements Argon2id password hashing/verification, the session
// mechanism, the progressive delay schedule, and the sweepers.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

// research §4: t=3, m=64MiB, p=2; 16-byte salt, 32-byte key. Parameters are
// encoded in the stored value so they can be raised without invalidating
// existing passwords.
var defaultParams = params{
	memory:      64 * 1024,
	iterations:  3,
	parallelism: 2,
	saltLength:  16,
	keyLength:   32,
}

// HashPassword returns an encoded Argon2id PHC string of the form
//
//	$argon2id$v=19$m=<mem>,t=<it>,p=<p>$<saltB64>$<keyB64>
func HashPassword(password string) (string, error) {
	p := defaultParams
	salt := make([]byte, p.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)
	b64s := base64.RawStdEncoding.EncodeToString(salt)
	b64k := base64.RawStdEncoding.EncodeToString(key)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.parallelism, b64s, b64k), nil
}

// VerifyPassword checks the password against the stored encoded value. The
// comparison is constant-time. Returns nil on match.
func VerifyPassword(password, encoded string) error {
	p, salt, key, err := parseEncoded(encoded)
	if err != nil {
		return err
	}
	want := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)
	if subtle.ConstantTimeCompare(want, key) != 1 {
		return errors.New("password mismatch")
	}
	return nil
}

func parseEncoded(s string) (params, []byte, []byte, error) {
	parts := strings.Split(s, "$")
	if len(parts) != 6 {
		return params{}, nil, nil, errors.New("malformed encoded hash")
	}
	if parts[1] != "argon2id" {
		return params{}, nil, nil, errors.New("unsupported variant")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return params{}, nil, nil, err
	}
	if version != argon2.Version {
		return params{}, nil, nil, errors.New("unsupported version")
	}
	var p params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return params{}, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return params{}, nil, nil, err
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return params{}, nil, nil, err
	}
	p.saltLength = uint32(len(salt))
	p.keyLength = uint32(len(key))
	return p, salt, key, nil
}
