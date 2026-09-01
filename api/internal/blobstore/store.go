// Package blobstore writes and reads attachment bodies in the configured
// store directory. The directory is outside the repository working tree
// (config.go refuses to start otherwise), files are named by the attachment's
// UUID with no extension, and they are sharded two levels deep so a directory
// listing stays small (research.md D-011, FR-012).
//
// The package deliberately knows nothing about encryption. Encryption is the
// caller's job — this code is the place where bytes go to live once they are
// already ciphertext, and the place where ciphertext is fetched from. The
// separation is what makes "no plaintext on disk" checkable in one place
// rather than argued across handlers.
package blobstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Store is the on-disk attachment store rooted at a configured absolute
// directory. The directory is created lazily when needed but is otherwise
// treated as opaque — files are placed by id and read by id, never by name.
type Store struct {
	Root string // absolute path; validated at config-load time
}

// New creates a Store rooted at the given directory. The directory MUST
// exist and be writable; config.go already enforces both at startup.
func New(root string) *Store {
	return &Store{Root: root}
}

// ErrNotFound means no file exists at the expected path. Distinct from a
// generic os.ErrNotExist so callers can branch without string-matching.
var ErrNotFound = errors.New("attachment body not found")

// pathFor computes the on-disk path for an attachment id. The first four hex
// characters form a two-level shard; the rest is the file name with no
// extension. For example, attachment id "abcdef01-2345-..." lives at
// <root>/ab/cd/abcdef01-2345-.... The path is computed but the file is not
// touched — Put and Open handle the actual I/O.
func (s *Store) pathFor(id uuid.UUID) (string, error) {
	hex := strings.ReplaceAll(id.String(), "-", "")
	if len(hex) < 4 {
		return "", fmt.Errorf("attachment id too short: %s", id)
	}
	shard1 := hex[:2]
	shard2 := hex[2:4]
	name := id.String()
	return filepath.Join(s.Root, shard1, shard2, name), nil
}

// PathFor exposes the path computation for callers that need to open the
// file directly (the upload handler streams into a freshly opened file
// rather than copying through Put). The path is computed but the file is
// not touched.
func (s *Store) PathFor(id uuid.UUID) (string, error) {
	return s.pathFor(id)
}

// Put writes the contents of src to a file at the attachment's path,
// creating intermediate directories as needed. The file is opened with
// 0600 permissions so the running process is the only reader; the parent
// directories inherit the umask. The caller is expected to have streamed
// ciphertext, not plaintext.
func (s *Store) Put(id uuid.UUID, src io.Reader) (string, error) {
	full, err := s.pathFor(id)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir store dir: %w", err)
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("open store file: %w", err)
	}
	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()
		_ = os.Remove(full)
		return "", fmt.Errorf("copy store file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(full)
		return "", fmt.Errorf("close store file: %w", err)
	}
	return full, nil
}

// Open returns a reader for the body of an attachment. The caller MUST close
// the file when done. The file does not exist returns ErrNotFound; any other
// error is wrapped.
func (s *Store) Open(id uuid.UUID) (io.ReadCloser, error) {
	full, err := s.pathFor(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("open store file: %w", err)
	}
	return f, nil
}

// Delete removes an attachment's file. A missing file is not an error:
// delete is called after the row is gone, and the store's contents should
// converge on "exactly one file per row". Any other I/O error is returned.
func (s *Store) Delete(id uuid.UUID) error {
	full, err := s.pathFor(id)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove store file: %w", err)
	}
	return nil
}
