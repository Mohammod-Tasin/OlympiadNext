// Package storage persists uploaded binary assets (event images and
// student KYC files) on the local filesystem and exposes them under a
// public URL prefix. It is a driven adapter: the HTTP layer depends on
// *LocalStorage directly since there is currently only one implementation.
package storage

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// URLPrefix is the path the static file server is mounted at in the
// router; Save returns URLs rooted here.
const URLPrefix = "/uploads"

// ErrUnsupportedType is returned by the Validate* helpers when a file's
// extension or sniffed content is not an accepted type. Handlers map it
// to 400.
var ErrUnsupportedType = errors.New("storage: unsupported file type")

// imageExts are the raster image formats every upload endpoint accepts.
var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

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

// Save streams src to a new file under subdir (relative to the uploads
// root; "" means the root itself), named with a random UUID that keeps
// the lower-cased extension of origName, and returns the public URL path
// (e.g. "/uploads/users/<id>/2b1c…e9.pdf"). subdir is cleaned so it can
// never escape the uploads root.
func (s *LocalStorage) Save(subdir, origName string, src io.Reader) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("storage: generate id failed: %w", err)
	}

	cleanSub := cleanSubdir(subdir)
	destDir := filepath.Join(s.dir, filepath.FromSlash(cleanSub))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("storage: create subdir failed: %w", err)
	}

	name := id + strings.ToLower(filepath.Ext(origName))
	dst, err := os.Create(filepath.Join(destDir, name))
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

	return path.Join(URLPrefix, cleanSub, name), nil
}

// UserFilePath resolves a file previously stored under "users/<ownerID>"
// to its on-disk path, rejecting any ownerID or name that is not a single
// clean path element (defence in depth against traversal).
func (s *LocalStorage) UserFilePath(ownerID, name string) (string, error) {
	if !isCleanSegment(ownerID) || !isCleanSegment(name) {
		return "", fmt.Errorf("storage: invalid file reference")
	}
	return filepath.Join(s.dir, "users", ownerID, name), nil
}

// ValidateImage checks that filename has an accepted image extension and
// that head (the first bytes of the file, ideally 512) actually sniffs as
// an image, so a renamed non-image is rejected.
func ValidateImage(filename string, head []byte) error {
	if !imageExts[strings.ToLower(filepath.Ext(filename))] {
		return fmt.Errorf("%w: allowed image types are jpg, jpeg, png, webp, gif", ErrUnsupportedType)
	}
	if !strings.HasPrefix(http.DetectContentType(head), "image/") {
		return fmt.Errorf("%w: file content is not a valid image", ErrUnsupportedType)
	}
	return nil
}

// ValidateDocument accepts the same images as ValidateImage plus PDF, for
// KYC proof documents which may be a scan or a phone photo.
func ValidateDocument(filename string, head []byte) error {
	if strings.ToLower(filepath.Ext(filename)) == ".pdf" {
		// http.DetectContentType returns "application/pdf" for the %PDF- magic.
		if http.DetectContentType(head) != "application/pdf" {
			return fmt.Errorf("%w: file content is not a valid PDF", ErrUnsupportedType)
		}
		return nil
	}
	return ValidateImage(filename, head)
}

// cleanSubdir normalises a caller-supplied subdirectory to a slash path
// that stays within the uploads root: prefixing "/" and cleaning collapses
// any "../" so the worst case is a path rooted at the uploads dir.
func cleanSubdir(subdir string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(filepath.ToSlash(subdir)))
	return strings.TrimPrefix(cleaned, "/")
}

// isCleanSegment reports whether s is a single, non-empty path element
// with no separator or parent reference.
func isCleanSegment(s string) bool {
	return s != "" && s == filepath.Base(s) && s != "." && s != ".." && !strings.ContainsAny(s, `/\`)
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
