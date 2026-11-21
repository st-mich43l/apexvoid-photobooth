package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrateOptions configures MigrateLibrary.
type MigrateOptions struct {
	// FreeBytes reports free space for the volume holding path (nil = unchecked).
	FreeBytes func(path string) int64
	// Progress receives cumulative bytes copied and the total.
	Progress func(done, total int64)
	// SrcDB is the already-open source database (used for a consistent snapshot).
	// If nil, the source database is opened temporarily.
	SrcDB *DB
}

// MigrateResult summarises a migration.
type MigrateResult struct {
	Dest       string   `json:"dest"`
	Source     string   `json:"source"`
	Files      int      `json:"files"`
	Skipped    int      `json:"skipped"`  // identical files already present (resumed run)
	Symlinks   []string `json:"symlinks"` // not copied (never followed)
	Bytes      int64    `json:"bytes"`
	Sessions   int      `json:"sessions"`
	MediaRows  int      `json:"mediaRows"`
	Verified   bool     `json:"verified"`
	SourceKept bool     `json:"sourceKept"`
}

// ConflictError is returned when a destination file exists with different content.
type ConflictError struct{ Path string }

func (e *ConflictError) Error() string {
	return "refusing to overwrite a different file in the new library: " + e.Path
}

// ErrDestHasLibrary: the destination already contains a library.
var ErrDestHasLibrary = errors.New("the destination already contains a media library")

const inProgressMarker = ".migration-in-progress"

// volatile files are written by the running application while it is being
// migrated (the daily log). They are copied best-effort and excluded from the
// strict size verification and from conflict detection.
func volatile(rel string) bool { return strings.HasPrefix(filepath.ToSlash(rel), "logs/") }

type srcFile struct {
	rel  string
	size int64
}

// nearestExisting returns the closest existing ancestor of p (for free-space checks).
func nearestExisting(p string) string {
	for {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p
		}
		p = parent
	}
}

func within(parent, child string) bool {
	r, err := filepath.Rel(parent, child)
	return err == nil && r != "." && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

// MigrateLibrary copies a library to dst without ever modifying or deleting
// the source, then verifies the copy. It is resumable (identical files are
// skipped, different ones are a conflict) and writes the database last, so a
// partial copy is never mistaken for a library.
func MigrateLibrary(ctx context.Context, src, dst Layout, o MigrateOptions) (MigrateResult, error) {
	res := MigrateResult{Dest: dst.Root, Source: src.Root, SourceKept: true}
	srcAbs, err1 := filepath.Abs(src.Root)
	dstAbs, err2 := filepath.Abs(dst.Root)
	if err1 != nil || err2 != nil {
		return res, errors.New("invalid library path")
	}
	if srcAbs == dstAbs || within(srcAbs, dstAbs) || within(dstAbs, srcAbs) {
		return res, errors.New("the new library folder must be separate from the old one (not inside it, and not containing it)")
	}
	if _, err := os.Stat(src.DatabasePath()); err != nil {
		return res, fmt.Errorf("no library found to migrate in %s", src.Root)
	}
	if _, err := os.Stat(dst.DatabasePath()); err == nil {
		return res, ErrDestHasLibrary
	}

	// 1. inventory the source (database handled separately; symlinks never followed)
	var files []srcFile
	var total int64
	walkErr := filepath.WalkDir(src.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src.Root, p)
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			res.Symlinks = append(res.Symlinks, filepath.ToSlash(rel))
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		base := filepath.Base(p)
		if strings.HasPrefix(base, ".tmp-") || base == inProgressMarker || base == ".write-test" || base == "MIGRATED_TO.txt" {
			return nil
		}
		slash := filepath.ToSlash(rel)
		if strings.HasPrefix(slash, "database/") { // photobooth.db(-wal/-shm): snapshot instead
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, srcFile{rel: rel, size: info.Size()})
		total += info.Size()
		return nil
	})
	if walkErr != nil {
		return res, fmt.Errorf("reading the old library: %w", walkErr)
	}
	var dbSize int64
	if st, err := os.Stat(src.DatabasePath()); err == nil {
		dbSize = st.Size()
	}

	// 2. disk space (checked before anything is created), then permissions
	if o.FreeBytes != nil {
		need := total + dbSize*2 + total/20 + 64<<20
		if free := o.FreeBytes(nearestExisting(dstAbs)); free >= 0 && free < need {
			return res, fmt.Errorf("not enough free disk space for the new library: %d MB needed, %d MB free", need>>20, free>>20)
		}
	}
	if err := os.MkdirAll(dstAbs, 0o755); err != nil {
		return res, fmt.Errorf("cannot create %s: %w", dst.Root, err)
	}
	if err := dst.Ensure(); err != nil {
		return res, fmt.Errorf("the new library folder is not usable: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dstAbs, inProgressMarker), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		return res, err
	}

	// 3. copy files (never overwrite a different file)
	var done int64
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		target := filepath.Join(dstAbs, f.rel)
		if !within(dstAbs, target) {
			return res, fmt.Errorf("unsafe path in the old library: %s", f.rel)
		}
		if volatile(f.rel) {
			if _, err := os.Lstat(target); err == nil {
				res.Skipped++
				continue
			}
		}
		skipped, err := copyNoOverwrite(filepath.Join(src.Root, f.rel), target)
		if err != nil {
			return res, err
		}
		if skipped {
			res.Skipped++
		} else {
			res.Files++
		}
		res.Bytes += f.size
		done += f.size
		if o.Progress != nil {
			o.Progress(done, total)
		}
	}

	// 4. database snapshot, written last
	sdb := o.SrcDB
	if sdb == nil {
		var err error
		if sdb, err = Open(src); err != nil {
			return res, fmt.Errorf("opening the old database: %w", err)
		}
		defer sdb.Close()
	}
	if err := sdb.Backup(dst.DatabasePath()); err != nil {
		return res, fmt.Errorf("copying the database: %w", err)
	}

	// 5. verify
	if err := verifyMigration(src, dst, files, sdb, &res); err != nil {
		// keep the (unverified) copy aside as evidence but make it unusable as a library
		_ = os.Rename(dst.DatabasePath(), dst.DatabasePath()+".unverified")
		return res, fmt.Errorf("verification of the new library failed (the old library is untouched): %w", err)
	}
	_ = os.Remove(filepath.Join(dstAbs, inProgressMarker))
	res.Verified = true
	return res, nil
}

// copyNoOverwrite copies src→dst via a temp file + fsync + rename. An existing
// identical file is skipped; a different one is a ConflictError.
func copyNoOverwrite(src, dst string) (skipped bool, err error) {
	if st, err := os.Lstat(dst); err == nil {
		same, herr := sameContent(src, dst, st)
		if herr != nil {
			return false, herr
		}
		if same {
			return true, nil
		}
		return false, &ConflictError{Path: dst}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	in, err := os.Open(src)
	if err != nil {
		return false, err
	}
	defer in.Close()
	return false, WriteNewFrom(dst, func(w io.Writer) error { _, err := io.Copy(w, in); return err })
}

func sameContent(a, b string, bst os.FileInfo) (bool, error) {
	ast, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	if ast.Size() != bst.Size() {
		return false, nil
	}
	ha, err := fileSum(a)
	if err != nil {
		return false, err
	}
	hb, err := fileSum(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

func fileSum(p string) ([32]byte, error) {
	var out [32]byte
	f, err := os.Open(p)
	if err != nil {
		return out, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return out, err
	}
	copy(out[:], h.Sum(nil))
	return out, nil
}

// verifyMigration: every source file exists in the copy with the same size; the
// copied database opens, has the same row counts, and every file its rows
// reference (that exists in the source) exists in the copy.
func verifyMigration(src, dst Layout, files []srcFile, sdb *DB, res *MigrateResult) error {
	for _, f := range files {
		if volatile(f.rel) {
			if _, err := os.Stat(filepath.Join(dst.Root, f.rel)); err != nil {
				return fmt.Errorf("missing in the copy: %s", filepath.ToSlash(f.rel))
			}
			continue
		}
		st, err := os.Stat(filepath.Join(dst.Root, f.rel))
		if err != nil {
			return fmt.Errorf("missing in the copy: %s", filepath.ToSlash(f.rel))
		}
		if st.Size() != f.size {
			return fmt.Errorf("size mismatch in the copy: %s", filepath.ToSlash(f.rel))
		}
	}
	ddb, err := sql.Open("sqlite", "file:"+dst.DatabasePath()+"?mode=ro")
	if err != nil {
		return err
	}
	defer ddb.Close()
	var chk string
	if err := ddb.QueryRow(`PRAGMA integrity_check`).Scan(&chk); err != nil || chk != "ok" {
		return fmt.Errorf("the copied database failed its integrity check (%s %v)", chk, err)
	}
	count := func(db interface {
		QueryRow(string, ...any) *sql.Row
	}, q string) (n int) {
		_ = db.QueryRow(q).Scan(&n)
		return
	}
	for _, q := range []string{`SELECT COUNT(*) FROM sessions`, `SELECT COUNT(*) FROM media`} {
		if a, b := count(sdb, q), count(ddb, q); a != b {
			return fmt.Errorf("row count differs for %q: %d in the old database, %d in the copy", q, a, b)
		}
	}
	res.Sessions, res.MediaRows = count(ddb, `SELECT COUNT(*) FROM sessions`), count(ddb, `SELECT COUNT(*) FROM media`)
	rows, err := ddb.Query(`SELECT id, rel_path, thumb_rel, preview_rel, edited_rel, status FROM media`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		var rels [4]string
		if err := rows.Scan(&id, &rels[0], &rels[1], &rels[2], &rels[3], &status); err != nil {
			return err
		}
		for _, rel := range rels {
			if rel == "" {
				continue
			}
			sp, err := src.Abs(rel)
			if err != nil {
				return fmt.Errorf("media %s references an unsafe path %q", id, rel)
			}
			dp, err := dst.Abs(rel)
			if err != nil {
				return fmt.Errorf("media %s references an unsafe path %q", id, rel)
			}
			if status == "trashed" { // trashed files live under trash/<id>/<rel>
				sp = filepath.Join(src.TrashDir(), id, filepath.FromSlash(rel))
				dp = filepath.Join(dst.TrashDir(), id, filepath.FromSlash(rel))
			}
			if _, err := os.Stat(sp); err != nil {
				continue // already missing in the source: not a migration defect
			}
			if _, err := os.Stat(dp); err != nil {
				return fmt.Errorf("media %s: %s did not arrive in the new library", id, rel)
			}
		}
	}
	return rows.Err()
}
