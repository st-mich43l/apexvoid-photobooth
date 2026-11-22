package templates

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// FRAME_DUMP=<dir> go test -run TestDumpFrames writes a PNG of every enabled
// frame (and the first variant of each design) for visual review.
func TestDumpFrames(t *testing.T) {
	dir := os.Getenv("FRAME_DUMP")
	if dir == "" {
		t.Skip("set FRAME_DUMP to a directory to dump previews")
	}
	_ = os.MkdirAll(dir, 0o755)
	for _, f := range Load(t.TempDir()).All() {
		if f.Legacy {
			continue
		}
		for _, v := range append([]string{""}, variantIDs(f)...) {
			if v != "" && f.LayoutID != "strip4" || os.Getenv("FRAME_ONLY") != "" && !strings.Contains(os.Getenv("FRAME_ONLY"), f.Design) {
				continue
			}
			r, err := f.WithVariant(v)
			if err != nil {
				t.Fatal(err)
			}
			img, err := Preview(r, nil, RenderContext{Caption: "", Date: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)}, 600)
			if err != nil {
				t.Fatalf("%s: %v", f.ID, err)
			}
			out, _ := os.Create(filepath.Join(dir, f.ID+"_"+v+".png"))
			_ = png.Encode(out, img)
			out.Close()
		}
	}
}

func variantIDs(f Template) []string {
	var out []string
	for _, v := range f.Variants {
		out = append(out, v.ID)
	}
	return out
}
