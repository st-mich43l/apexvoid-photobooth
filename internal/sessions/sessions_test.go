package sessions

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

func jpegBytes(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	b, err := media.EncodeJPEG(img, 90)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func open(t *testing.T, root string) (*Store, *storage.DB) {
	t.Helper()
	db, err := storage.Open(storage.Layout{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db), db
}

func TestSessionLifecycleAndPersistence(t *testing.T) {
	root := t.TempDir()
	s, db := open(t, root)
	sess, err := s.Create("photo", 4, "strip", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range storage.SessionSubdirs {
		if st, err := os.Stat(filepath.Join(root, "sessions", sess.ID, d)); err != nil || !st.IsDir() {
			t.Fatalf("missing session dir %s", d)
		}
	}
	data := jpegBytes(t, 300, 200, color.NRGBA{200, 50, 50, 255})
	m, err := s.AddPhoto(sess.ID, PhotoInput{Slot: 0, Data: data, Source: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Width != 300 || m.Height != 200 || m.ThumbRel == "" || m.PreviewRel == "" {
		t.Fatalf("bad media %+v", m)
	}
	orig, _ := db.Layout.Abs(m.RelPath)
	got, _ := os.ReadFile(orig)
	if !bytes.Equal(got, data) {
		t.Fatal("original differs from camera bytes")
	}
	if err := s.SetStatus(sess.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// reopen: everything persisted
	s2, _ := open(t, root)
	list, err := s2.List(10, 0)
	if err != nil || len(list) != 1 || list[0].Status != StatusCompleted || list[0].MediaCount != 1 {
		t.Fatalf("persistence failed: %+v %v", list, err)
	}
	ms, _ := s2.ListMedia(sess.ID, 0, 0)
	if len(ms) != 1 || ms[0].ID != m.ID {
		t.Fatalf("media not persisted: %+v", ms)
	}
}

func TestRetakeKeepsOldOriginalOnDisk(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, _ := s.Create("photo", 2, "", false)
	a, _ := s.AddPhoto(sess.ID, PhotoInput{Slot: 0, Data: jpegBytes(t, 64, 64, color.NRGBA{255, 0, 0, 255})})
	b, _ := s.AddPhoto(sess.ID, PhotoInput{Slot: 0, Data: jpegBytes(t, 64, 64, color.NRGBA{0, 255, 0, 255})})
	if err := s.DiscardMedia(a.ID); err != nil {
		t.Fatal(err)
	}
	ms, _ := s.ListMedia(sess.ID, 0, 0)
	if len(ms) != 1 || ms[0].ID != b.ID {
		t.Fatalf("only the retake should be visible: %+v", ms)
	}
	old, _ := db.Layout.Abs(a.RelPath)
	if _, err := os.Stat(old); err != nil {
		t.Fatal("discarded original was deleted")
	}
}

func TestEditsNeverTouchOriginal(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, _ := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 200, 100, color.NRGBA{90, 120, 200, 255})})
	orig, _ := db.Layout.Abs(m.RelPath)
	before, _ := os.ReadFile(orig)
	sum := sha256.Sum256(before)

	m2, err := s.SaveEdits(m.ID, media.Edits{Rotate: 90, Brightness: 0.3})
	if err != nil || m2.EditedRel == "" {
		t.Fatalf("SaveEdits: %v %+v", err, m2)
	}
	after, _ := os.ReadFile(orig)
	if sha256.Sum256(after) != sum {
		t.Fatal("original modified by edit")
	}
	ed, _ := os.ReadFile(mustAbs(t, db, m2.EditedRel))
	if w, h, _ := media.Dimensions(ed); w != 100 || h != 200 {
		t.Fatalf("edited render is %dx%d, want rotated 100x200", w, h)
	}
	// second edit replaces only the derived file
	m3, _ := s.SaveEdits(m.ID, media.Edits{Contrast: 0.2})
	if _, err := os.Stat(mustAbs(t, db, m2.EditedRel)); !os.IsNotExist(err) {
		t.Fatal("stale edited render should be removed")
	}
	// revert
	m4, _ := s.SaveEdits(m.ID, media.Edits{})
	if m4.EditedRel != "" {
		t.Fatal("revert should clear edit")
	}
	if _, err := os.Stat(mustAbs(t, db, m3.EditedRel)); !os.IsNotExist(err) {
		t.Fatal("reverted edit file should be removed")
	}
	if _, err := s.SaveEdits(m.ID, media.Edits{Brightness: 5}); err == nil {
		t.Fatal("invalid edit accepted")
	}
}

func mustAbs(t *testing.T, db *storage.DB, rel string) string {
	t.Helper()
	p, err := db.Layout.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExportNeverOverwrites(t *testing.T) {
	s, _ := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, _ := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 50, 50, color.NRGBA{1, 2, 3, 255})})
	dest := t.TempDir()
	p1, err := s.Export(m.ID, dest, false)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.Export(m.ID, dest, false)
	if err != nil || p1 == p2 {
		t.Fatalf("second export must pick a new name: %v %s %s", err, p1, p2)
	}
}

func TestTrashRestoreAndEmpty(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, _ := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 50, 50, color.NRGBA{1, 2, 3, 255})})
	if err := s.Trash(m.ID); err != nil {
		t.Fatal(err)
	}
	if ms, _ := s.ListMedia("", 0, 0); len(ms) != 0 {
		t.Fatal("trashed media still listed")
	}
	if _, err := s.FilePath(m.ID, "original"); err == nil {
		t.Fatal("trashed media must not be servable")
	}
	if _, err := os.Stat(mustAbs(t, db, m.RelPath)); !os.IsNotExist(err) {
		t.Fatal("file should have moved to trash")
	}
	if err := s.Restore(m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mustAbs(t, db, m.RelPath)); err != nil {
		t.Fatal("restore did not bring file back")
	}
	_ = s.Trash(m.ID)
	if n, err := s.EmptyTrash(); err != nil || n != 1 {
		t.Fatalf("EmptyTrash=%d %v", n, err)
	}
}

func TestVerifyRecoversOrphansAndInterrupted(t *testing.T) {
	root := t.TempDir()
	s, db := open(t, root)
	sess, _ := s.Create("photo", 1, "", false)
	m, _ := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 50, 50, color.NRGBA{9, 9, 9, 255})})

	// simulate: crash after the original hit the disk but before the row
	orphan := filepath.Join(root, "sessions", sess.ID, "originals", "IMG_02_orphan.jpg")
	if err := storage.WriteNew(orphan, jpegBytes(t, 80, 60, color.NRGBA{5, 5, 5, 255})); err != nil {
		t.Fatal(err)
	}
	// and a lost file for a known row
	_ = os.Remove(mustAbs(t, db, m.RelPath))

	rep, err := s.Verify(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Interrupted) != 1 || len(rep.Recovered) != 1 || len(rep.Missing) != 1 {
		t.Fatalf("report %+v", rep)
	}
	got, _ := s.Session(sess.ID)
	if got.Status != StatusInterrupted {
		t.Fatalf("status %s", got.Status)
	}
	// idempotent
	rep2, _ := s.Verify(false)
	if len(rep2.Recovered) != 0 {
		t.Fatalf("recovered twice: %+v", rep2)
	}
}

func TestRejectsUndecodableCameraData(t *testing.T) {
	s, _ := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	if _, err := s.AddPhoto(sess.ID, PhotoInput{Data: []byte("not an image")}); err == nil {
		t.Fatal("garbage accepted as photo")
	}
}

func TestWriteNewRefusesOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jpg")
	if err := storage.WriteNew(p, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteNew(p, []byte("two")); err == nil {
		t.Fatal("overwrite allowed")
	}
	if b, _ := os.ReadFile(p); string(b) != "one" {
		t.Fatal("content changed")
	}
}

func TestLayoutAbsBlocksTraversal(t *testing.T) {
	l := storage.Layout{Root: t.TempDir()}
	for _, bad := range []string{"../x", "sessions/../../x", "/etc/passwd", ""} {
		if _, err := l.Abs(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if _, err := l.Abs("sessions/S1/originals/a.jpg"); err != nil {
		t.Fatal(err)
	}
}

func TestBasePreviewIgnoresEdits(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, _ := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 200, 100, color.NRGBA{90, 120, 200, 255})})
	if _, err := s.SaveEdits(m.ID, media.Edits{Rotate: 90}); err != nil {
		t.Fatal(err)
	}
	base, _ := s.FilePath(m.ID, "basepreview")
	prev, _ := s.FilePath(m.ID, "preview")
	if base == prev {
		t.Fatal("editor base must differ from the edited preview")
	}
	if want, _ := db.Layout.Abs(m.PreviewRel); base != want {
		t.Fatalf("%s != %s", base, want)
	}
}
