package video

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func gz(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write(data)
	_ = w.Close()
	return b.Bytes()
}

func TestExtractEmbeddedIsIdempotentAndRepairsDamage(t *testing.T) {
	payload := bytes.Repeat([]byte("MZ-fake-ffmpeg-"), 5000)
	z := gz(t, payload)
	dir := filepath.Join(t.TempDir(), "Tài liệu", "bin") // non-ASCII path
	if gzipSize(z) != int64(len(payload)) {
		t.Fatalf("gzip trailer size %d vs %d", gzipSize(z), len(payload))
	}
	exe, err := extractEmbedded(dir, z, []byte("LGPL text"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	if !bytes.Equal(b, payload) {
		t.Fatal("unpacked content differs")
	}
	if l, _ := os.ReadFile(filepath.Join(dir, "FFMPEG-LICENSE.txt")); string(l) != "LGPL text" {
		t.Fatal("licence not written next to the executable")
	}
	st1, _ := os.Stat(exe)
	time.Sleep(20 * time.Millisecond)
	exe2, err := extractEmbedded(dir, z, nil)
	if err != nil || exe2 != exe {
		t.Fatal(err)
	}
	if st2, _ := os.Stat(exe); !st2.ModTime().Equal(st1.ModTime()) {
		t.Fatal("an intact copy must not be rewritten on every start")
	}
	// truncated by a crash / antivirus → rebuilt
	_ = os.Truncate(exe, 100)
	if _, err := extractEmbedded(dir, z, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); !bytes.Equal(b, payload) {
		t.Fatal("damaged copy was not repaired")
	}
	// no temp files left behind
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".unpack-") {
			t.Fatalf("leftover %s", e.Name())
		}
	}
}

func TestExtractEmbeddedRejectsBadData(t *testing.T) {
	if _, err := extractEmbedded(t.TempDir(), nil, nil); err == nil {
		t.Fatal("empty data accepted")
	}
	if _, err := extractEmbedded(t.TempDir(), []byte("not gzip at all, but long enough"), nil); err == nil {
		t.Fatal("corrupt data accepted")
	}
	z := gz(t, []byte("hello"))
	z[len(z)-4] = 0x7f // trailer says a different size than the stream holds
	if _, err := extractEmbedded(t.TempDir(), z, nil); err == nil {
		t.Fatal("size mismatch accepted")
	}
}

func TestConcurrentStartsUnpackOnce(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 200000)
	z := gz(t, payload)
	dir := t.TempDir()
	var wg sync.WaitGroup
	paths := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := extractEmbedded(dir, z, nil)
			if err != nil {
				t.Error(err)
			}
			paths <- p
		}()
	}
	wg.Wait()
	close(paths)
	first := ""
	for p := range paths {
		if first == "" {
			first = p
		}
		if p != first {
			t.Fatal("different paths")
		}
	}
	if b, _ := os.ReadFile(first); !bytes.Equal(b, payload) {
		t.Fatal("content")
	}
}

func TestNormalBuildHasNoBuiltInFFmpeg(t *testing.T) {
	if _, _, ok := embeddedFFmpeg(); ok {
		t.Skip("built with embedffmpeg")
	}
	if p, err := builtInFFmpeg(); p != "" || err != nil {
		t.Fatalf("%q %v", p, err)
	}
	if IsBuiltIn("/anything") {
		t.Fatal("IsBuiltIn must be false without embedded data")
	}
}
