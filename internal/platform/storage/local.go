// Package storage persists uploaded binary assets (event images) on the
// local filesystem and exposes them under a public URL prefix. It is a
// driven adapter: the HTTP layer depends on *LocalStorage directly since
// there is currently only one implementation.
package storage

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// URLPrefix is the path the static file server is mounted at in the
// router; Save returns URLs rooted here.
const URLPrefix = "/uploads"

type LocalStorage struct {
	dir string // absolute or working-dir-relative path to the uploads folder
}

// NewLocalStorage ensures dir exists (creating it on startup if needed)
// and returns a storage rooted there.
func NewLocalStorage(dir string) (*LocalStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create %q failed: %w", dir, err)
	}
	return &LocalStorage{dir: dir}, nil
}

// Dir is the filesystem path the static file server should serve.
func (s *LocalStorage) Dir() string { return s.dir }

// Save streams src to a new file named with a random UUID, keeping the
// lower-cased extension of origName, and returns the public URL path
// (e.g. "/uploads/2b1c...e9.png").
func (s *LocalStorage) Save(origName string, src io.Reader) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("storage: generate id failed: %w", err)
	}

	name := id + strings.ToLower(filepath.Ext(origName))
	dst, err := os.Create(filepath.Join(s.dir, name))
	if err != nil {
		return "", fmt.Errorf("storage: create file failed: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("storage: write file failed: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("storage: finalize file failed: %w", err)
	}

	return URLPrefix + "/" + name, nil
}

// newUUID returns a random RFC 4122 version-4 UUID string. Kept local so
// the project stays free of a UUID dependency.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
