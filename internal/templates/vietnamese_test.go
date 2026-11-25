package templates

import (
	"bytes"
	"testing"
)

func TestVietnameseFontCoversAccents(t *testing.T) {
	f := face("vietnamese", 24)
	if f == nil {
		t.Fatal("Noto Sans missing")
	}
	defer f.Close()
	for _, r := range "Tiếng Việt Ảnh cưới Đặng Nguyễn ắ ằ ẩ ễ ộ ự ỹ" {
		if _, ok := f.GlyphAdvance(r); !ok {
			t.Errorf("missing glyph %q", r)
		}
	}
}

func TestVietnameseCaptionRendersAccents(t *testing.T) {
	const caption = "Tiếng Việt — Ảnh cưới Đặng Nguyễn"
	if got := ExpandText("{caption}", RenderContext{Caption: caption}); got != caption {
		t.Fatalf("caption was changed: %q", got)
	}
	f := *Load(t.TempDir()).Get("white-minimal.strip4")
	accented, err := Preview(f, nil, RenderContext{Caption: caption}, 600)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Preview(f, nil, RenderContext{Caption: "Tieng Viet — Anh cuoi Dang Nguyen"}, 600)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(accented.Pix, plain.Pix) {
		t.Fatal("accented caption did not affect rendered pixels")
	}
}
