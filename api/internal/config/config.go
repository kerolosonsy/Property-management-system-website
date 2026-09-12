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
	"strings"
)

type Config struct {
	DatabaseAppURL      string
	DatabaseOwnerURL    string
	SessionCookieName   string
	TLSCertPath         string
	TLSKeyPath          string
	ListenAddr          string
	WebDist             string // optional absolute path of the production web bundle
	AttachmentStore     string // absolute path of attachment store directory
	AttachmentMaxBytes  int64  // configured upload size limit, in bytes
	ExtractTextMaxBytes int64  // configured extracted-text cap, in bytes
	FieldKEK            []byte // 32 bytes; raw form of PMS_KEK after base64 decode
}

const (
	defaultAttachmentMaxBytes  = 50 * 1024 * 1024 // FR-005 default: 50 MB
	defaultExtractTextMaxBytes = 256 * 1024       // FR-021 default: 256 KB
)

func Load() (*Config, error) {
	c := &Config{
		DatabaseAppURL:      envOr("PMS_DATABASE_APP_URL", ""),
		DatabaseOwnerURL:    envOr("PMS_DATABASE_OWNER_URL", ""),
		SessionCookieName:   envOr("PMS_SESSION_COOKIE_NAME", "pms_session"),
		TLSCertPath:         envOr("PMS_TLS_CERT_PATH", ""),
		TLSKeyPath:          envOr("PMS_TLS_KEY_PATH", ""),
		ListenAddr:          envOr("PMS_LISTEN_ADDR", ":8443"),
		WebDist:             envOr("PMS_WEB_DIST", ""),
		AttachmentStore:     envOr("PMS_ATTACHMENT_STORE", ""),
		AttachmentMaxBytes:  parseInt64(envOr("PMS_ATTACHMENT_MAX_BYTES", ""), defaultAttachmentMaxBytes),
		ExtractTextMaxBytes: parseInt64(envOr("PMS_EXTRACT_TEXT_MAX_BYTES", ""), defaultExtractTextMaxBytes),
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

	if err := validateAttachmentStore(c.AttachmentStore); err != nil {
		return nil, err
	}
	if err := validateWebDist(c.WebDist); err != nil {
		return nil, err
	}
	if c.WebDist != "" {
		if err := validateWebDistSeparation(c.WebDist, c.AttachmentStore); err != nil {
			return nil, err
		}
	}
	if c.AttachmentMaxBytes <= 0 {
		return nil, errors.New("PMS_ATTACHMENT_MAX_BYTES must be a positive integer")
	}
	if c.ExtractTextMaxBytes <= 0 {
		return nil, errors.New("PMS_EXTRACT_TEXT_MAX_BYTES must be a positive integer")
	}

	kek, err := loadKEK(envOr("PMS_KEK", ""))
	if err != nil {
		return nil, err
	}
	c.FieldKEK = kek

	return c, nil
}

// validateWebDist leaves the existing API-only deployment unchanged when the
// setting is empty. When configured, the server must be able to serve a real
// production bundle rather than silently falling back to an invalid path.
func validateWebDist(path string) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("PMS_WEB_DIST must be an absolute path; got %q", path)
	}
	dirInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("PMS_WEB_DIST is not usable: %w", err)
	}
	if !dirInfo.IsDir() {
		return fmt.Errorf("PMS_WEB_DIST must be a directory; %q is not", path)
	}
	return nil
}

func validateWebDistSeparation(webDist, attachmentStore string) error {
	resolvedWebDist, err := filepath.EvalSymlinks(webDist)
	if err != nil {
		return fmt.Errorf("resolve PMS_WEB_DIST: %w", err)
	}
	resolvedAttachmentStore, err := filepath.EvalSymlinks(attachmentStore)
	if err != nil {
		return fmt.Errorf("resolve PMS_ATTACHMENT_STORE: %w", err)
	}
	if pathContains(resolvedWebDist, resolvedAttachmentStore) || pathContains(resolvedAttachmentStore, resolvedWebDist) {
		// Static serving must never make encrypted attachment blobs reachable
		// without the authenticated attachment handler.
		return errors.New("PMS_WEB_DIST and PMS_ATTACHMENT_STORE must not overlap")
	}
	return nil
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// validateAttachmentStore refuses to start when the store path is missing,
// not writable, or sits inside the repository working tree (FR-012, FR-016).
func validateAttachmentStore(path string) error {
	if path == "" {
		return errors.New("PMS_ATTACHMENT_STORE is required (absolute path of the attachment store directory)")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("PMS_ATTACHMENT_STORE must be an absolute path; got %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("PMS_ATTACHMENT_STORE is not usable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("PMS_ATTACHMENT_STORE must be a directory; %q is not", path)
	}
	// Writable by this process: try to create and remove a probe file.
	probe := filepath.Join(path, ".pms-write-probe")
	f, err := os.Create(probe)
	if err != nil {
		return fmt.Errorf("PMS_ATTACHMENT_STORE is not writable: %w", err)
	}
	_ = f.Close()
	_ = os.Remove(probe)

	// Refuse when the store sits inside the repo working tree. Comparing the
	// resolved path against the process's working directory catches the obvious
	// mistake of pointing it at a subdirectory.
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve PMS_ATTACHMENT_STORE: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("read working directory: %w", err)
	}
	rel, err := filepath.Rel(cwd, abs)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != ".." {
		return fmt.Errorf("PMS_ATTACHMENT_STORE must be outside the repository working tree; %q is inside %q", abs, cwd)
	}
	return nil
}

// parseInt64 parses a decimal integer or returns the default. Empty strings and
// parse errors fall back to the default rather than refusing to start — the
// defaults are documented in the .env.example, and a configured value of 0 or
// negative is caught later as an invalid setting.
func parseInt64(raw string, def int64) int64 {
	if raw == "" {
		return def
	}
	n := int64(0)
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int64(ch-'0')
	}
	if n <= 0 {
		return def
	}
	return n
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
