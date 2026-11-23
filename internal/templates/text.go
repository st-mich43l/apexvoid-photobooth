package templates

import (
	_ "embed"
	"image"
	"image/color"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/norm"
)

// Existing Go fonts preserve prior layout. Noto Sans is used for text the
// selected font cannot render, when Noto covers every rune in that text.
//
//go:embed fonts/NotoSans-Regular.ttf
var notoSans []byte

var (
	fontOnce sync.Once
	fontSet  map[string]*opentype.Font
)

func loadFonts() {
	fontSet = map[string]*opentype.Font{}
	for name, ttf := range map[string][]byte{"sans": goregular.TTF, "bold": gobold.TTF, "italic": goitalic.TTF, "mono": gomono.TTF, "vietnamese": notoSans} {
		if f, err := opentype.Parse(ttf); err == nil {
			fontSet[name] = f
		}
	}
}

func face(name string, px float64) font.Face {
	fontOnce.Do(loadFonts)
	f := fontSet[name]
	if f == nil {
		f = fontSet["sans"]
	}
	if f == nil {
		return nil
	}
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	return fc
}

// ExpandText substitutes {caption}, {date} and {a|b|literal} (first non-empty).
func ExpandText(s string, rc RenderContext) string {
	var out strings.Builder
	for {
		i := strings.IndexByte(s, '{')
		j := strings.IndexByte(s, '}')
		if i < 0 || j < i {
			out.WriteString(s)
			break
		}
		out.WriteString(s[:i])
		for _, part := range strings.Split(s[i+1:j], "|") {
			v := part
			switch strings.TrimSpace(part) {
			case "caption":
				v = strings.TrimSpace(rc.Caption)
			case "date":
				v = ""
				if !rc.Date.IsZero() {
					v = rc.Date.Format("2006.01.02")
				}
			case "time":
				v = ""
				if !rc.Date.IsZero() {
					v = rc.Date.Format("15:04")
				}
			}
			if v != "" {
				out.WriteString(v)
				break
			}
		}
		s = s[j+1:]
	}
	return out.String()
}

func drawText(canvas *image.NRGBA, t Template, l Layer, rect image.Rectangle, px func(float64) int, col color.NRGBA, rc RenderContext) {
	// Normalize only the rendered string; the saved caption and old images stay unchanged.
	txt := norm.NFC.String(ExpandText(l.Text, rc))
	if strings.TrimSpace(txt) == "" || rect.Dx() <= 0 {
		return
	}
	size := l.SizeIn * float64(t.DPI) // em height in pixels
	fontName := l.Font
	if primary, fallback := face(fontName, size), face("vietnamese", size); primary != nil && fallback != nil {
		missing, covered := false, true
		for _, r := range txt {
			if _, ok := primary.GlyphAdvance(r); !ok {
				missing = true
			}
			if _, ok := fallback.GlyphAdvance(r); !ok {
				covered = false
			}
		}
		_ = primary.Close()
		_ = fallback.Close()
		if missing && covered {
			fontName = "vietnamese"
		}
	} else {
		if primary != nil {
			_ = primary.Close()
		}
		if fallback != nil {
			_ = fallback.Close()
		}
	}
	var fc font.Face
	// shrink the text until it fits the layer width (never below 40 %)
	var width fixed.Int26_6
	for k := 0; k < 12; k++ {
		if fc != nil {
			_ = fc.Close()
		}
		if fc = face(fontName, size); fc == nil {
			return
		}
		width = font.MeasureString(fc, txt)
		if width.Ceil() <= rect.Dx() || size <= l.SizeIn*float64(t.DPI)*0.4 {
			break
		}
		size *= 0.92
	}
	defer fc.Close()
	m := fc.Metrics()
	x := rect.Min.X
	switch l.Align {
	case "center", "":
		x = rect.Min.X + (rect.Dx()-width.Ceil())/2
	case "right":
		x = rect.Max.X - width.Ceil()
	}
	h := m.Ascent + m.Descent
	baseline := fixed.I(rect.Min.Y) + (fixed.I(rect.Dy())-h)/2 + m.Ascent
	d := font.Drawer{Dst: canvas, Src: image.NewUniform(col), Face: fc, Dot: fixed.Point26_6{X: fixed.I(x), Y: baseline}}
	d.DrawString(txt)
}
