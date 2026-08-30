// Package config loads environment variables and validates them. The server
// refuses to start when a required variable is missing or unusable (FR-031,
// Constitution VII).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DatabaseAppURL   string
	DatabaseOwnerURL string
	SessionCookieName string
	TLSCertPath      string
	TLSKeyPath       string
	ListenAddr       string
	AttachmentKEK    string // reserved; not exercised by this feature
	FieldKEK         []byte // 32 bytes; raw form of PMS_KEK after base64 decode
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseAppURL:    envOr("PMS_DATABASE_APP_URL", ""),
		DatabaseOwnerURL:  envOr("PMS_DATABASE_OWNER_URL", ""),
		SessionCookieName: envOr("PMS_SESSION_COOKIE_NAME", "pms_session"),
		TLSCertPath:       envOr("PMS_TLS_CERT_PATH", ""),
		TLSKeyPath:        envOr("PMS_TLS_KEY_PATH", ""),
		ListenAddr:        envOr("PMS_LISTEN_ADDR", ":8443"),
		AttachmentKEK:     envOr("PMS_ATTACHMENT_KEK", ""),
	}

	if c.DatabaseAppURL == "" {
		return nil, errors.New("PMS_DATABASE_APP_URL is required")
	}
	if c.TLSCertPath == "" || c.TLSKeyPath == "" {
		return nil, errors.New("PMS_TLS_CERT_PATH and PMS_TLS_KEY_PATH are required (TLS only, FR-029)")
	}
	if !fileExists(c.TLSCertPath) {
		return nil, fmt.Errorf("TLS certificate not found at %s", c.TLSCertPath)
	}
	if !fileExists(c.TLSKeyPath) {
		return nil, fmt.Errorf("TLS key not found at %s", c.TLSKeyPath)
	}

	kek, err := loadKEK(envOr("PMS_KEK", ""))
	if err != nil {
		return nil, err
	}
	c.FieldKEK = kek

	return c, nil
}

// loadKEK decodes the base64-encoded PMS_KEK and verifies it is exactly 32 bytes
// (Constitution VII). The server refuses to start when it is missing or wrong.
func loadKEK(raw string) ([]byte, error) {
	if raw == "" {
		return nil, errors.New("PMS_KEK is required (32 random bytes, base64). Generate with: openssl rand -base64 32")
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("PMS_KEK is not valid base64: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("PMS_KEK must decode to exactly 32 bytes; got %d", len(b))
	}
	return b, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	_, err = os.Stat(abs)
	return err == nil
}
