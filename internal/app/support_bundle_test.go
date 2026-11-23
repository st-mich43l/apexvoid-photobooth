package app

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingLogIsBounded(t *testing.T) {
	dir := t.TempDir()
	l, err := openRotatingLog(dir, 128, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if _, err = l.Write([]byte(strings.Repeat("s", 60) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 3 {
		t.Fatalf("too many files: %d", len(entries))
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.Size() > 128 {
			t.Fatalf("unbounded %s: %v", e.Name(), err)
		}
	}
}
func TestSupportBundleNeverExportsRawLogData(t *testing.T) {
	s, root, _ := newTestApp(t)
	connectMock(t, s)
	secret := "private-user-secret-91387"
	s.log.Error("print failed", "printer", secret, "path", root)
	folder := t.TempDir()
	path, err := s.ExportSupportBundle(folder)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var all strings.Builder
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	for _, bad := range []string{secret, root, filepath.Base(root), s.settings.Path} {
		if strings.Contains(all.String(), bad) {
			t.Fatalf("bundle leaks %q", bad)
		}
	}
	if !strings.Contains(all.String(), "print_failure") || !strings.Contains(all.String(), "UNVERIFIED") {
		t.Fatalf("missing sanitized evidence: %s", all.String())
	}
}
