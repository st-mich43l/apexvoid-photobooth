package templates

import "testing"

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
