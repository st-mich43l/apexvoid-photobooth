package app

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

func TestRepeatedStartupShutdownReleasesLibraryAndPreviewPort(t *testing.T) {
	root := t.TempDir()
	opts := Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), SkipRestore: true, ResolveLibrary: func(*settings.Manager) settings.Resolution { return settings.Resolution{Path: root} }}
	for cycle := 0; cycle < 5; cycle++ {
		s, err := New(opts)
		if err != nil {
			t.Fatalf("startup %d: %v", cycle, err)
		}
		addr := strings.TrimPrefix(s.httpd.base, "http://")
		if err := s.Shutdown(); err != nil {
			t.Fatalf("shutdown %d: %v", cycle, err)
		}
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			t.Fatalf("preview port still listening after shutdown %d", cycle)
		}
	}
}
