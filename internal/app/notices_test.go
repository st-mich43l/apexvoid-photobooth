package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteThirdPartyNoticesDoesNotReplaceExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notices.txt")
	if err := WriteThirdPartyNotices(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "SIL OPEN FONT LICENSE") || !strings.Contains(string(b), "FFmpeg") {
		t.Fatal("expected bundled notices")
	}
	if err = WriteThirdPartyNotices(path); err == nil {
		t.Fatal("existing notices were overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(b) {
		t.Fatal("existing file changed")
	}
}
