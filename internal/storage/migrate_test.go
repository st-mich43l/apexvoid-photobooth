package storage_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

func jpegOf(t *testing.T, c uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 80, 60))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c, 100, 200, 255
	}
	b, err := media.EncodeJPEG(img, 90)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// buildLegacy creates a realistic library: sessions, photos, an edit, a
// trashed photo, a video file, plus logs.
func buildLegacy(t *testing.T, root string) (*storage.DB, map[string]bool) {
	t.Helper()
	db, err := storage.Open(storage.Layout{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	s := sessions.New(db)
	ids := map[string]bool{}
	for i := 0; i < 2; i++ {
		sess, _ := s.Create("photo", 2, "strip-2x6-2", false)
		for slot := 0; slot < 2; slot++ {
			m, err := s.AddPhoto(sess.ID, sessions.PhotoInput{Slot: slot, Data: jpegOf(t, uint8(40*i+slot*10)), Source: "test"})
			if err != nil {
				t.Fatal(err)
			}
			ids[m.ID] = true
		}
		_ = s.SetStatus(sess.ID, sessions.StatusCompleted, "")
	}
	ms, _ := s.ListMedia("", 10, 0)
	if _, err := s.SaveEdits(ms[0].ID, media.Edits{Rotate: 90}); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ms[1].ID); err != nil {
		t.Fatal(err)
	}
	vs, _ := s.Create("video", 1, "", false)
	vid, vpath, _ := s.NewVideoPath(vs.ID)
	_ = os.WriteFile(vpath, bytes.Repeat([]byte("v"), 4096), 0o644)
	if _, err := s.AddVideo(vs.ID, vid, sessions.VideoInput{AbsPath: vpath, DurationMs: 1000, Source: "t"}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "logs", "app-20261101.log"), []byte("log"), 0o644)
	return db, ids
}

// tree maps every file (except the database) to its sha256.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if strings.HasPrefix(filepath.ToSlash(rel), "database/") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		b, _ := os.ReadFile(p)
		h := sha256.Sum256(b)
		out[filepath.ToSlash(rel)] = string(h[:])
		return nil
	})
	return out
}

func sameTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestMigrationCopiesVerifiesAndKeepsSource(t *testing.T) {
	base := t.TempDir()
	srcRoot := filepath.Join(base, "Nguyễn Văn", "AppData", "Local", "ApexVoidPhotobooth") // Unicode user name
	dstRoot := filepath.Join(base, "OneDrive – Tài liệu", "apexvoid-studio")
	sdb, _ := buildLegacy(t, srcRoot)
	defer sdb.Close()
	before := tree(t, srcRoot)

	var last int64
	res, err := storage.MigrateLibrary(context.Background(), storage.Layout{Root: srcRoot}, storage.Layout{Root: dstRoot},
		storage.MigrateOptions{SrcDB: sdb, Progress: func(done, total int64) { last = done }})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || !res.SourceKept || res.Sessions != 3 || res.MediaRows != 5 || res.Files == 0 || last == 0 {
		t.Fatalf("%+v", res)
	}
	if !sameTree(before, tree(t, srcRoot)) {
		t.Fatal("the source library was modified")
	}
	if !sameTree(before, tree(t, dstRoot)) {
		t.Fatal("copied files differ from the source")
	}
	if _, err := os.Stat(filepath.Join(dstRoot, ".migration-in-progress")); err == nil {
		t.Fatal("in-progress marker left behind")
	}
	// the new library works with the normal repository code
	ndb, err := storage.Open(storage.Layout{Root: dstRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer ndb.Close()
	ns := sessions.New(ndb)
	ms, _ := ns.ListMedia("", 50, 0)
	if len(ms) != 4 { // 4 photos - 1 trashed + 1 video
		t.Fatalf("visible media %d", len(ms))
	}
	for _, m := range ms {
		p, err := ns.FilePath(m.ID, "original")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("referenced file missing in the new library: %v", err)
		}
		if !strings.HasPrefix(p, dstRoot) {
			t.Fatalf("new library still points at the old location: %s", p)
		}
	}
}

func TestMigrationIsResumableAndIdempotent(t *testing.T) {
	base := t.TempDir()
	src, dst := filepath.Join(base, "old"), filepath.Join(base, "new")
	sdb, _ := buildLegacy(t, src)
	defer sdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	_, err := storage.MigrateLibrary(ctx, storage.Layout{Root: src}, storage.Layout{Root: dst}, storage.MigrateOptions{SrcDB: sdb,
		Progress: func(int64, int64) {
			n++
			if n == 3 {
				cancel()
			}
		}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "database", "photobooth.db")); err == nil {
		t.Fatal("an interrupted copy must not look like a library (database is written last)")
	}
	res, err := storage.MigrateLibrary(context.Background(), storage.Layout{Root: src}, storage.Layout{Root: dst}, storage.MigrateOptions{SrcDB: sdb})
	if err != nil || !res.Verified || res.Skipped == 0 {
		t.Fatalf("resume failed: %v %+v", err, res)
	}
}

func TestMigrationRefusesToOverwriteDifferentFiles(t *testing.T) {
	base := t.TempDir()
	src, dst := filepath.Join(base, "old"), filepath.Join(base, "new")
	sdb, _ := buildLegacy(t, src)
	defer sdb.Close()
	var victim string
	for k := range tree(t, src) {
		if strings.HasPrefix(k, "sessions/") && strings.Contains(k, "originals") {
			victim = k
			break
		}
	}
	vp := filepath.Join(dst, filepath.FromSlash(victim))
	_ = os.MkdirAll(filepath.Dir(vp), 0o755)
	_ = os.WriteFile(vp, []byte("someone else's file"), 0o644)
	_, err := storage.MigrateLibrary(context.Background(), storage.Layout{Root: src}, storage.Layout{Root: dst}, storage.MigrateOptions{SrcDB: sdb})
	var ce *storage.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
	if b, _ := os.ReadFile(vp); string(b) != "someone else's file" {
		t.Fatal("existing file was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dst, "database", "photobooth.db")); err == nil {
		t.Fatal("database written despite the conflict")
	}
}

func TestMigrationGuards(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "old")
	sdb, _ := buildLegacy(t, src)
	defer sdb.Close()
	ctx := context.Background()
	l := func(p string) storage.Layout { return storage.Layout{Root: p} }

	if _, err := storage.MigrateLibrary(ctx, l(src), l(filepath.Join(src, "inside")), storage.MigrateOptions{SrcDB: sdb}); err == nil {
		t.Fatal("destination inside the source accepted")
	}
	if _, err := storage.MigrateLibrary(ctx, l(src), l(base), storage.MigrateOptions{SrcDB: sdb}); err == nil {
		t.Fatal("destination containing the source accepted")
	}
	if _, err := storage.MigrateLibrary(ctx, l(src), l(src), storage.MigrateOptions{SrcDB: sdb}); err == nil {
		t.Fatal("same folder accepted")
	}
	if _, err := storage.MigrateLibrary(ctx, l(filepath.Join(base, "nothing")), l(filepath.Join(base, "x")), storage.MigrateOptions{}); err == nil {
		t.Fatal("missing source accepted")
	}
	full := filepath.Join(base, "full")
	if _, err := storage.MigrateLibrary(ctx, l(src), l(full), storage.MigrateOptions{SrcDB: sdb, FreeBytes: func(string) int64 { return 1 << 10 }}); err == nil || !strings.Contains(err.Error(), "not enough free disk space") {
		t.Fatalf("space check: %v", err)
	}
	if _, err := os.Stat(filepath.Join(full, "database")); err == nil {
		t.Fatal("nothing may be written when space is insufficient")
	}
	existing := filepath.Join(base, "existing")
	edb, _ := storage.Open(l(existing))
	edb.Close()
	if _, err := storage.MigrateLibrary(ctx, l(src), l(existing), storage.MigrateOptions{SrcDB: sdb}); !errors.Is(err, storage.ErrDestHasLibrary) {
		t.Fatalf("existing destination library: %v", err)
	}
	// unwritable destination (a file where the folder should be)
	blocker := filepath.Join(base, "blocker")
	_ = os.WriteFile(blocker, []byte("x"), 0o644)
	if _, err := storage.MigrateLibrary(ctx, l(src), l(filepath.Join(blocker, "lib")), storage.MigrateOptions{SrcDB: sdb}); err == nil {
		t.Fatal("unwritable destination accepted")
	}
}

func TestMigrationDoesNotFollowSymlinks(t *testing.T) {
	base := t.TempDir()
	src, dst := filepath.Join(base, "old"), filepath.Join(base, "new")
	sdb, _ := buildLegacy(t, src)
	defer sdb.Close()
	secret := filepath.Join(base, "secret.txt")
	_ = os.WriteFile(secret, []byte("do not copy"), 0o644)
	if err := os.Symlink(secret, filepath.Join(src, "sessions", "link.txt")); err != nil {
		t.Skip("symlinks unavailable: ", err)
	}
	res, err := storage.MigrateLibrary(context.Background(), storage.Layout{Root: src}, storage.Layout{Root: dst}, storage.MigrateOptions{SrcDB: sdb})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Symlinks) != 1 {
		t.Fatalf("%+v", res.Symlinks)
	}
	if _, err := os.Stat(filepath.Join(dst, "sessions", "link.txt")); err == nil {
		t.Fatal("symlink target was copied")
	}
}

func TestMigrationRejectsPoisonedDatabasePaths(t *testing.T) {
	base := t.TempDir()
	src, dst := filepath.Join(base, "old"), filepath.Join(base, "new")
	sdb, _ := buildLegacy(t, src)
	defer sdb.Close()
	if _, err := sdb.Exec(`UPDATE media SET rel_path='../../outside.jpg' WHERE id=(SELECT id FROM media LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	_, err := storage.MigrateLibrary(context.Background(), storage.Layout{Root: src}, storage.Layout{Root: dst}, storage.MigrateOptions{SrcDB: sdb})
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("path traversal in database rows must fail verification: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "database", "photobooth.db")); err == nil {
		t.Fatal("unverified database left in place as a usable library")
	}
	var names []string
	for k := range tree(t, src) {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("source emptied")
	}
}
