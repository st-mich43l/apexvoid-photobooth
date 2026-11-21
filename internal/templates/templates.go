// Package templates composes photo strips and sheets at an exact physical
// size and DPI: cropping/positioning, backgrounds, borders and overlays.
package templates

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

// Slot is a photo position in inches from the top-left of the canvas.
type Slot struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	Photo  int     `json:"photo"`  // index into the session's photos (repeated for 2-up layouts)
	FocusX float64 `json:"focusX"` // 0..1 crop anchor, 0 means centre (0.5)
	FocusY float64 `json:"focusY"`
}

// Template is a print layout.
type Template struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"` // strip | sheet | single | custom
	WidthIn    float64 `json:"widthIn"`
	HeightIn   float64 `json:"heightIn"`
	DPI        int     `json:"dpi"`
	Background string  `json:"background"` // #RRGGBB
	BorderIn   float64 `json:"borderIn"`   // frame drawn around every slot
	BorderCol  string  `json:"borderColor"`
	Overlay    string  `json:"overlay"` // PNG file name inside the templates dir (custom frames)
	Slots      []Slot  `json:"slots"`
}

// Photos is how many distinct photos the template consumes.
func (t Template) Photos() int {
	n := 0
	for _, s := range t.Slots {
		if s.Photo+1 > n {
			n = s.Photo + 1
		}
	}
	return n
}

// PixelSize returns the exact output size in pixels.
func (t Template) PixelSize() (int, int) {
	return int(math.Round(t.WidthIn * float64(t.DPI))), int(math.Round(t.HeightIn * float64(t.DPI)))
}

// Validate checks the template is renderable.
func (t Template) Validate() error {
	if t.ID == "" || strings.ContainsAny(t.ID, `/\ `) {
		return errors.New("template id must be non-empty without spaces or slashes")
	}
	if t.DPI < 72 || t.DPI > 600 {
		return errors.New("dpi must be 72..600")
	}
	if t.WidthIn <= 0 || t.HeightIn <= 0 || t.WidthIn > 24 || t.HeightIn > 36 {
		return errors.New("invalid physical size")
	}
	if w, h := t.PixelSize(); w*h > 80_000_000 {
		return errors.New("canvas too large (> 80 megapixels)")
	}
	if len(t.Slots) == 0 {
		return errors.New("template has no photo slots")
	}
	for i, s := range t.Slots {
		if s.W <= 0 || s.H <= 0 || s.X < 0 || s.Y < 0 || s.X+s.W > t.WidthIn+1e-6 || s.Y+s.H > t.HeightIn+1e-6 {
			return fmt.Errorf("slot %d lies outside the canvas", i+1)
		}
		if s.Photo < 0 {
			return fmt.Errorf("slot %d has a negative photo index", i+1)
		}
	}
	for _, c := range []string{t.Background, t.BorderCol} {
		if c != "" {
			if _, err := ParseColor(c); err != nil {
				return err
			}
		}
	}
	return nil
}

// ParseColor parses #RRGGBB.
func ParseColor(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("color %q must be #RRGGBB", s)
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.NRGBA{}, fmt.Errorf("color %q must be #RRGGBB", s)
	}
	return color.NRGBA{r, g, b, 255}, nil
}

func stripSlots(n int, w, top, gap, slotH float64) []Slot {
	var out []Slot
	for i := 0; i < n; i++ {
		out = append(out, Slot{X: 0.15, Y: top + float64(i)*(slotH+gap), W: w - 0.3, H: slotH, Photo: i})
	}
	return out
}

func grid(cols, rows int, w, h, margin, gap float64, footer float64) []Slot {
	cw := (w - 2*margin - float64(cols-1)*gap) / float64(cols)
	ch := (h - 2*margin - footer - float64(rows-1)*gap) / float64(rows)
	var out []Slot
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			out = append(out, Slot{X: margin + float64(c)*(cw+gap), Y: margin + float64(r)*(ch+gap), W: cw, H: ch, Photo: r*cols + c})
		}
	}
	return out
}

// Builtin returns the shipped layouts: Korean-style vertical strips, 2x6 and
// 4x6 formats, single-photo layouts.
func Builtin() []Template {
	two := func(base []Slot) []Slot { // two identical 2x6 strips on one 4x6 sheet (cut in half)
		out := append([]Slot{}, base...)
		for _, s := range base {
			s.X += 2
			out = append(out, s)
		}
		return out
	}
	s4 := stripSlots(4, 2, 0.15, 0.1, 1.2)
	return []Template{
		{ID: "strip-2x6-4", Name: "Korean strip 2×6 — 4 photos", Kind: "strip", WidthIn: 2, HeightIn: 6, DPI: 300, Background: "#14101f", BorderIn: 0.02, BorderCol: "#f4a6c0", Slots: s4},
		{ID: "strip-2x6-3", Name: "Strip 2×6 — 3 photos", Kind: "strip", WidthIn: 2, HeightIn: 6, DPI: 300, Background: "#ffffff", Slots: stripSlots(3, 2, 0.15, 0.1, 1.6)},
		{ID: "strip-2x6-2", Name: "Strip 2×6 — 2 photos", Kind: "strip", WidthIn: 2, HeightIn: 6, DPI: 300, Background: "#fdf2f6", Slots: stripSlots(2, 2, 0.15, 0.12, 2.4)},
		{ID: "strip-4x6-2up", Name: "4×6 sheet — two 2×6 strips (cut apart)", Kind: "sheet", WidthIn: 4, HeightIn: 6, DPI: 300, Background: "#14101f", BorderIn: 0.02, BorderCol: "#f4a6c0", Slots: two(s4)},
		{ID: "sheet-4x6-6", Name: "4×6 sheet — 6 photos", Kind: "sheet", WidthIn: 4, HeightIn: 6, DPI: 300, Background: "#ffffff", Slots: grid(2, 3, 4, 6, 0.15, 0.1, 0.3)},
		{ID: "single-4x6", Name: "Single photo 6×4 landscape", Kind: "single", WidthIn: 6, HeightIn: 4, DPI: 300, Background: "#ffffff", Slots: []Slot{{X: 0.15, Y: 0.15, W: 5.7, H: 3.7}}},
		{ID: "single-4x6-portrait", Name: "Single photo 4×6 portrait", Kind: "single", WidthIn: 4, HeightIn: 6, DPI: 300, Background: "#ffffff", Slots: []Slot{{X: 0.15, Y: 0.15, W: 3.7, H: 5.7}}},
	}
}

// ForShots picks the default builtin for a shot count.
func ForShots(n int) string {
	switch n {
	case 1:
		return "single-4x6"
	case 2:
		return "strip-2x6-2"
	case 3:
		return "strip-2x6-3"
	case 6:
		return "sheet-4x6-6"
	}
	return "strip-2x6-4"
}

// Library is the set of builtin plus user templates (JSON files in dir).
type Library struct {
	Dir  string
	list []Template
	Errs []string // problems loading user templates, shown in the UI
}

// Load reads builtins and user templates from dir (created if missing).
func Load(dir string) *Library {
	l := &Library{Dir: dir, list: Builtin()}
	_ = os.MkdirAll(dir, 0o755)
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		var t Template
		if err := json.Unmarshal(b, &t); err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		if t.Kind == "" {
			t.Kind = "custom"
		}
		if err := t.Validate(); err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		if l.Get(t.ID) != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("%s: id %q already exists", filepath.Base(f), t.ID))
			continue
		}
		l.list = append(l.list, t)
	}
	return l
}

// All returns every template.
func (l *Library) All() []Template { return append([]Template{}, l.list...) }

// Get returns a template by id (nil if unknown).
func (l *Library) Get(id string) *Template {
	for i := range l.list {
		if l.list[i].ID == id {
			t := l.list[i]
			return &t
		}
	}
	return nil
}

// Compose renders photos into the template at exact pixel size.
func (l *Library) Compose(t Template, photos []image.Image) (*image.NRGBA, error) {
	return Compose(t, photos, l.Dir)
}

// Compose renders photos into t. overlayDir resolves t.Overlay.
func Compose(t Template, photos []image.Image, overlayDir string) (*image.NRGBA, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if len(photos) < t.Photos() {
		return nil, fmt.Errorf("template %q needs %d photos, got %d", t.ID, t.Photos(), len(photos))
	}
	cw, ch := t.PixelSize()
	bg := color.NRGBA{255, 255, 255, 255}
	if t.Background != "" {
		bg, _ = ParseColor(t.Background)
	}
	canvas := media.Solid(cw, ch, bg)
	dpi := float64(t.DPI)
	px := func(in float64) int { return int(math.Round(in * dpi)) }
	for _, s := range t.Slots {
		r := image.Rect(px(s.X), px(s.Y), px(s.X+s.W), px(s.Y+s.H))
		if t.BorderIn > 0 {
			bc := color.NRGBA{255, 255, 255, 255}
			if t.BorderCol != "" {
				bc, _ = ParseColor(t.BorderCol)
			}
			b := px(t.BorderIn)
			draw.Draw(canvas, r, image.NewUniform(bc), image.Point{}, draw.Src)
			r = r.Inset(b)
		}
		fx, fy := s.FocusX, s.FocusY
		if fx == 0 {
			fx = 0.5
		}
		if fy == 0 {
			fy = 0.5
		}
		cover(canvas, r, photos[s.Photo], fx, fy)
	}
	if t.Overlay != "" {
		if err := drawOverlay(canvas, filepath.Join(overlayDir, filepath.Base(t.Overlay))); err != nil {
			return nil, err
		}
	}
	return canvas, nil
}

// cover scales src to fill r (cropping the excess around the focus point).
func cover(dst *image.NRGBA, r image.Rectangle, src image.Image, fx, fy float64) {
	sb := src.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	scale := math.Max(float64(r.Dx())/sw, float64(r.Dy())/sh)
	cw, chh := float64(r.Dx())/scale, float64(r.Dy())/scale // crop size in source pixels
	x0 := float64(sb.Min.X) + (sw-cw)*fx
	y0 := float64(sb.Min.Y) + (sh-chh)*fy
	crop := image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x0+cw)), int(math.Round(y0+chh))).Intersect(sb)
	xdraw.CatmullRom.Scale(dst, r, src, crop, xdraw.Src, nil)
}

func drawOverlay(canvas *image.NRGBA, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("overlay: %w", err)
	}
	defer f.Close()
	ov, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("overlay is not a PNG: %w", err)
	}
	xdraw.CatmullRom.Scale(canvas, canvas.Bounds(), ov, ov.Bounds(), xdraw.Over, nil)
	return nil
}

// Render composes and encodes a JPEG with the template DPI in its header.
func Render(t Template, photos []image.Image, overlayDir string) (data []byte, w, h int, err error) {
	img, err := Compose(t, photos, overlayDir)
	if err != nil {
		return nil, 0, 0, err
	}
	enc, err := media.EncodeJPEG(img, 95)
	if err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	return media.SetJPEGDensity(enc, t.DPI), b.Dx(), b.Dy(), nil
}
