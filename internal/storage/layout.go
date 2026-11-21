// Package storage owns the on-disk layout (SQLite metadata + media files) and
// crash-safe file writes. It knows nothing about cameras or the UI.
package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Layout is the media-library directory structure:
//
//	<root>/database/photobooth.db
//	<root>/sessions/<id>/{originals,previews,edited,videos,exports}
//	<root>/templates/  <root>/logs/  <root>/trash/
type Layout struct{ Root string }

// SessionSubdirs are created for every session.
var SessionSubdirs = []string{"originals", "previews", "edited", "videos", "exports"}

func (l Layout) DatabaseDir() string  { return filepath.Join(l.Root, "database") }
func (l Layout) DatabasePath() string { return filepath.Join(l.DatabaseDir(), "photobooth.db") }
func (l Layout) SessionsDir() string  { return filepath.Join(l.Root, "sessions") }
func (l Layout) TemplatesDir() string { return filepath.Join(l.Root, "templates") }
func (l Layout) LogsDir() string      { return filepath.Join(l.Root, "logs") }
func (l Layout) TrashDir() string     { return filepath.Join(l.Root, "trash") }
func (l Layout) SessionDir(id string) string {
	return filepath.Join(l.SessionsDir(), id)
}

// Abs resolves a library-relative path and refuses anything that escapes the
// root (defence against corrupted database rows).
func (l Layout) Abs(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid library path %q", rel)
	}
	p := filepath.Join(l.Root, filepath.FromSlash(rel))
	r, err := filepath.Rel(l.Root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("library path %q escapes root", rel)
	}
	return p, nil
}

// Rel converts an absolute path inside the library to a slash-separated
// relative path stored in the database.
func (l Layout) Rel(abs string) (string, error) {
	r, err := filepath.Rel(l.Root, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is outside the library", abs)
	}
	return filepath.ToSlash(r), nil
}

// Ensure creates the top level directories and verifies the root is writable.
func (l Layout) Ensure() error {
	for _, d := range []string{l.DatabaseDir(), l.SessionsDir(), l.TemplatesDir(), l.LogsDir(), l.TrashDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	probe := filepath.Join(l.Root, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("media folder %s is not writable: %w", l.Root, err)
	}
	return os.Remove(probe)
}

// EnsureSession creates the session directory tree.
func (l Layout) EnsureSession(id string) error {
	for _, s := range SessionSubdirs {
		if err := os.MkdirAll(filepath.Join(l.SessionDir(id), s), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// ErrExists is returned by WriteNew when the destination already exists.
var ErrExists = errors.New("file already exists")

// WriteNew writes data to path atomically (temp file + fsync + rename) and
// refuses to overwrite an existing file, so originals are never replaced.
func WriteNew(path string, data []byte) error {
	return WriteNewFrom(path, func(w io.Writer) error { _, err := w.Write(data); return err })
}

// WriteNewFrom is WriteNew with a streaming writer.
func WriteNewFrom(path string, fill func(io.Writer) error) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(name) }
	if err := fill(tmp); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		_ = os.Remove(name)
		return fmt.Errorf("%w: %s", ErrExists, path)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// WriteReplace atomically replaces path (for settings and derived files only,
// never for originals).
func WriteReplace(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}
