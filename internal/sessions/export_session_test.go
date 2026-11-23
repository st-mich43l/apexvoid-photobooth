package sessions

import (
	"archive/zip"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportSessionIsLocalSafeAndNeverOverwrites(t *testing.T) {
	s, _ := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, err := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 10, 10, color.NRGBA{R: 1, G: 2, B: 3, A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DiscardMedia(m.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SetStatus(sess.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	first, err := s.ExportSession(sess.ID, dest, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ExportSession(sess.ID, dest, true)
	if err != nil || first == second {
		t.Fatalf("overwrite %s %s %v", first, second, err)
	}
	z, err := zip.OpenReader(first)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	found := false
	for _, f := range z.File {
		if strings.Contains(f.Name, "..") || strings.HasPrefix(f.Name, "/") {
			t.Fatalf("unsafe ZIP name %s", f.Name)
		}
		if strings.HasPrefix(f.Name, "originals/") {
			found = true
		}
	}
	if !found {
		t.Fatal("discarded original omitted")
	}
	if _, err = os.Stat(filepath.Join(dest, "nonexistent")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestExportSessionRejectsOtherSessionFilesAndRemovesPartial(t *testing.T) {
	s, db := open(t, t.TempDir())
	a, _ := s.Create("photo", 1, "", false)
	b, _ := s.Create("photo", 1, "", false)
	other, err := s.AddPhoto(b.ID, PhotoInput{Data: jpegBytes(t, 10, 10, color.NRGBA{R: 3, G: 2, B: 1, A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO media(id,session_id,kind,rel_path,captured_at,created_at) VALUES('bad',?,'photo',?,'t','t')`, a.ID, other.RelPath); err != nil {
		t.Fatal(err)
	}
	if err = s.SetStatus(a.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err = s.ExportSession(a.ID, dest, false); err == nil {
		t.Fatal("another session's file was exported")
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial archive remains: %v %v", entries, err)
	}
}
