// Package templates is the frame library: print layouts (photo-slot geometry)
// plus their visual styling (colours, borders, artwork, decorations, text),
// and the single renderer that composes photos into them.
//
// A frame is a Template. Its JSON is flat, but in Go it is split in two so a
// layout can be reused by many designs:
//
//   - Geometry: physical size, DPI and photo slots (where photos go);
//   - Style: background, borders, corner rounding, artwork and layers (how it looks).
//
// All lengths are inches from the top-left of the canvas, so one frame renders
// identically at any DPI (thumbnail, preview, 300-DPI print). The Go renderer
// is authoritative: every preview in the UI is rendered by it, never by a
// second implementation.
package templates

import (
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"regexp"
	"strings"
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
	Locked bool    `json:"locked,omitempty"` // editor only: keep the slot where it is
}

// Geometry is the layout: where photos go on a canvas of a physical size.
type Geometry struct {
	WidthIn  float64 `json:"widthIn"`
	HeightIn float64 `json:"heightIn"`
	DPI      int     `json:"dpi"`
	Slots    []Slot  `json:"slots"`
}

// Layer kinds.
const (
	LayerText    = "text"
	LayerRect    = "rect"
	LayerEllipse = "ellipse"
	LayerShape   = "shape"
	LayerPattern = "pattern"
	LayerImage   = "image"
)

// Shapes drawable by shape/pattern layers.
var Shapes = []string{"heart", "star", "sparkle", "flower", "cloud", "triangle", "dot", "square", "sprocket", "diamond"}

// Fonts available to text layers (embedded; no system fonts are used so the
// result is identical on every machine).
var Fonts = []string{"sans", "bold", "italic", "mono"}

// Layer is one decoration, text or image on top of (or under) the photos.
type Layer struct {
	ID      string  `json:"id"`
	Name    string  `json:"name,omitempty"`
	Kind    string  `json:"kind"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	W       float64 `json:"w"`
	H       float64 `json:"h"`
	Color   string  `json:"color,omitempty"`   // #RRGGBB or a token: bg, accent, text, border
	Opacity float64 `json:"opacity,omitempty"` // 0 means 1
	Under   bool    `json:"under,omitempty"`   // drawn beneath the photos
	Hidden  bool    `json:"hidden,omitempty"`
	Locked  bool    `json:"locked,omitempty"`

	StrokeIn float64 `json:"strokeIn,omitempty"` // rect/ellipse: outline width instead of a fill
	RadiusIn float64 `json:"radiusIn,omitempty"` // rect: corner radius

	Shape  string   `json:"shape,omitempty"`  // shape, and the element of a pattern
	ElemH  float64  `json:"elemH,omitempty"`  // pattern element height (0 = same as Size)
	Size   float64  `json:"size,omitempty"`   // pattern element width
	Gap    float64  `json:"gap,omitempty"`    // pattern spacing
	Spread bool     `json:"spread,omitempty"` // pattern: scattered (confetti) instead of a grid
	Colors []string `json:"colors,omitempty"` // pattern: colours cycled over the elements

	Text   string  `json:"text,omitempty"`   // may contain {caption}, {date} or {caption|date|literal}
	Font   string  `json:"font,omitempty"`   // one of Fonts
	SizeIn float64 `json:"sizeIn,omitempty"` // text height (em) in inches
	Align  string  `json:"align,omitempty"`  // left | center | right

	Asset string `json:"asset,omitempty"` // image layer: file in the frame's assets
	Fit   string `json:"fit,omitempty"`   // image layer: contain (default) | cover
}

// Variant is a colour option of a design: it overrides some style colours.
type Variant struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Background  string `json:"background,omitempty"`
	BorderColor string `json:"borderColor,omitempty"`
	Accent      string `json:"accent,omitempty"`
	TextColor   string `json:"textColor,omitempty"`
}

// Swatch is the colour shown for the variant in pickers.
func (v Variant) Swatch(base Template) string {
	switch {
	case v.Background != "":
		return v.Background
	case v.Accent != "":
		return v.Accent
	}
	return base.Background
}

// Style is how a frame looks.
type Style struct {
	Background      string    `json:"background"`                // canvas colour #RRGGBB
	BorderIn        float64   `json:"borderIn"`                  // frame drawn around every slot
	BorderCol       string    `json:"borderColor"`               // #RRGGBB or token
	CornerIn        float64   `json:"cornerIn,omitempty"`        // rounded photo corners
	Accent          string    `json:"accent,omitempty"`          // token "accent"
	TextColor       string    `json:"textColor,omitempty"`       // token "text"
	BackgroundImage string    `json:"backgroundImage,omitempty"` // artwork under the photos (asset name)
	Overlay         string    `json:"overlay"`                   // transparent PNG over the photos (asset name)
	Layers          []Layer   `json:"layers,omitempty"`
	Variants        []Variant `json:"variants,omitempty"`
}

// Frame categories shown in the Booth.
const (
	CatClassic = "Classic"
	CatPastel  = "Korean Pastel"
	CatRetro   = "Retro"
	CatSpecial = "Special"
	CatPlain   = "Plain"
	CatCustom  = "Custom"
	CatLegacy  = "Legacy"
)

// Categories lists the categories in display order.
var Categories = []string{CatClassic, CatPastel, CatRetro, CatSpecial, CatPlain, CatCustom}

// Template is a frame: catalogue metadata + geometry + style. Its JSON is flat
// and backward compatible with the earlier layout files.
type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"` // strip | sheet | single | custom
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
	Order       int    `json:"order,omitempty"`
	Revision    int    `json:"revision,omitempty"`
	Enabled     bool   `json:"enabled"`                  // offered in Booth Mode
	Builtin     bool   `json:"builtin,omitempty"`        // shipped with the app (never persisted)
	Legacy      bool   `json:"legacy,omitempty"`         // an earlier layout kept so old sessions still resolve
	Caption     bool   `json:"captionEnabled,omitempty"` // guests may type a short caption
	Design      string `json:"design,omitempty"`         // design family id (built-ins: one design on many layouts)
	LayoutID    string `json:"layoutId,omitempty"`
	VariantID   string `json:"variantId,omitempty"` // set on a resolved copy (see WithVariant)

	Geometry
	Style
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

// Format is a short human label such as "2×6 strip" or "4×6 sheet".
func (t Template) Format() string {
	w, h := t.WidthIn, t.HeightIn
	if w > h {
		w, h = h, w
	}
	size := fmt.Sprintf("%g×%g", round2(w), round2(h))
	switch {
	case t.Photos() == 1:
		return size + " single"
	case w <= 2.5:
		return size + " strip"
	}
	return size + " sheet"
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// Clone returns a deep copy (slices are not shared with the library).
func (t Template) Clone() Template {
	c := t
	c.Slots = append([]Slot(nil), t.Slots...)
	c.Layers = make([]Layer, len(t.Layers))
	for i, l := range t.Layers {
		l.Colors = append([]string(nil), l.Colors...)
		c.Layers[i] = l
	}
	if len(t.Layers) == 0 {
		c.Layers = nil
	}
	c.Variants = append([]Variant(nil), t.Variants...)
	return c
}

// WithVariant returns a copy with the colour variant applied ("" = the frame as designed).
func (t Template) WithVariant(id string) (Template, error) {
	c := t.Clone()
	if id == "" {
		return c, nil
	}
	for _, v := range t.Variants {
		if v.ID != id {
			continue
		}
		if v.Background != "" {
			c.Background = v.Background
		}
		if v.BorderColor != "" {
			c.BorderCol = v.BorderColor
		}
		if v.Accent != "" {
			c.Accent = v.Accent
		}
		if v.TextColor != "" {
			c.TextColor = v.TextColor
		}
		c.VariantID = id
		return c, nil
	}
	return Template{}, fmt.Errorf("frame %q has no colour option %q", t.ID, id)
}

var (
	idRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	assetRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}\.(png|jpg|jpeg)$`)
)

// ValidID reports whether id is usable as a frame id (also a folder name).
func ValidID(id string) bool { return idRe.MatchString(id) }

// ValidAsset reports whether name is an acceptable image asset file name.
func ValidAsset(name string) bool { return assetRe.MatchString(strings.ToLower(name)) }

const (
	maxSlots  = 24
	maxLayers = 64
)

// Validate checks the template is renderable and safe.
func (t Template) Validate() error {
	if !ValidID(t.ID) {
		return errors.New("frame id must be 1-64 letters, digits, dots, dashes or underscores")
	}
	if t.DPI < 72 || t.DPI > 600 {
		return errors.New("dpi must be 72..600")
	}
	if !(t.WidthIn > 0 && t.HeightIn > 0) || t.WidthIn > 24 || t.HeightIn > 36 {
		return errors.New("invalid physical size")
	}
	if w, h := t.PixelSize(); w*h > 80_000_000 {
		return errors.New("canvas too large (> 80 megapixels)")
	}
	if len(t.Slots) == 0 {
		return errors.New("template has no photo slots")
	}
	if len(t.Slots) > maxSlots {
		return fmt.Errorf("too many photo slots (max %d)", maxSlots)
	}
	used := map[int]bool{}
	for i, s := range t.Slots {
		if !(s.W > 0 && s.H > 0) || s.X < 0 || s.Y < 0 || s.X+s.W > t.WidthIn+1e-6 || s.Y+s.H > t.HeightIn+1e-6 {
			return fmt.Errorf("slot %d lies outside the canvas", i+1)
		}
		if s.Photo < 0 || s.Photo >= maxSlots {
			return fmt.Errorf("slot %d has an invalid photo index", i+1)
		}
		if s.FocusX < 0 || s.FocusX > 1 || s.FocusY < 0 || s.FocusY > 1 {
			return fmt.Errorf("slot %d focus must be 0..1", i+1)
		}
		used[s.Photo] = true
	}
	for p := 0; p < t.Photos(); p++ {
		if !used[p] {
			return fmt.Errorf("photo %d is not used by any slot (photo numbers must run 1..%d without gaps)", p+1, t.Photos())
		}
	}
	if t.BorderIn < 0 || t.CornerIn < 0 || t.BorderIn > 1 || t.CornerIn > 2 {
		return errors.New("border or corner size out of range")
	}
	for _, c := range []string{t.Background, t.BorderCol, t.Accent, t.TextColor} {
		if c != "" && !isToken(c) {
			if _, err := ParseColor(c); err != nil {
				return err
			}
		}
	}
	for _, a := range []string{t.BackgroundImage, t.Overlay} {
		if a != "" && !ValidAsset(a) {
			return fmt.Errorf("image %q is not a valid PNG/JPEG file name", a)
		}
	}
	if len(t.Layers) > maxLayers {
		return fmt.Errorf("too many layers (max %d)", maxLayers)
	}
	ids := map[string]bool{}
	for i, l := range t.Layers {
		if err := l.validate(); err != nil {
			return fmt.Errorf("layer %d: %w", i+1, err)
		}
		if l.ID != "" {
			if ids[l.ID] {
				return fmt.Errorf("layer %d: duplicate id %q", i+1, l.ID)
			}
			ids[l.ID] = true
		}
	}
	vids := map[string]bool{}
	for _, v := range t.Variants {
		if !ValidID(v.ID) || v.Name == "" || vids[v.ID] {
			return fmt.Errorf("invalid or duplicate colour option %q", v.ID)
		}
		vids[v.ID] = true
		for _, c := range []string{v.Background, v.BorderColor, v.Accent, v.TextColor} {
			if c != "" {
				if _, err := ParseColor(c); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func finite(fs ...float64) bool {
	for _, f := range fs {
		if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 1000 {
			return false
		}
	}
	return true
}

func (l Layer) validate() error {
	if !finite(l.X, l.Y, l.W, l.H, l.StrokeIn, l.RadiusIn, l.Size, l.ElemH, l.Gap, l.SizeIn, l.Opacity) || l.W < 0 || l.H < 0 {
		return errors.New("invalid position or size")
	}
	if l.Opacity < 0 || l.Opacity > 1 {
		return errors.New("opacity must be 0..1")
	}
	if l.Color != "" && !isToken(l.Color) {
		if _, err := ParseColor(l.Color); err != nil {
			return err
		}
	}
	switch l.Kind {
	case LayerRect, LayerEllipse:
	case LayerShape:
		if !contains(Shapes, l.Shape) {
			return fmt.Errorf("unknown shape %q", l.Shape)
		}
	case LayerPattern:
		if !contains(Shapes, l.Shape) {
			return fmt.Errorf("unknown pattern element %q", l.Shape)
		}
		if l.Size <= 0.01 {
			return errors.New("pattern elements are too small")
		}
		eh := l.ElemH
		if eh == 0 {
			eh = l.Size
		}
		if l.W > 0 && l.H > 0 && (int(l.W/(l.Size+l.Gap))+1)*(int(l.H/(eh+l.Gap))+1) > 4000 {
			return errors.New("pattern has too many elements")
		}
		for _, c := range l.Colors {
			if _, err := ParseColor(c); err != nil && !isToken(c) {
				return err
			}
		}
	case LayerText:
		if len(l.Text) > 200 {
			return errors.New("text is too long (max 200 characters)")
		}
		if l.Font != "" && !contains(Fonts, l.Font) {
			return fmt.Errorf("unknown font %q", l.Font)
		}
		if l.SizeIn <= 0 || l.SizeIn > 12 {
			return errors.New("text size must be 0..12 inches")
		}
		if l.Align != "" && l.Align != "left" && l.Align != "center" && l.Align != "right" {
			return errors.New("align must be left, center or right")
		}
	case LayerImage:
		if !ValidAsset(l.Asset) {
			return fmt.Errorf("image %q is not a valid PNG/JPEG file name", l.Asset)
		}
		if l.Fit != "" && l.Fit != "contain" && l.Fit != "cover" {
			return errors.New("fit must be contain or cover")
		}
	default:
		return fmt.Errorf("unknown layer kind %q", l.Kind)
	}
	return nil
}

func isToken(s string) bool {
	switch s {
	case "bg", "accent", "text", "border":
		return true
	}
	return false
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

// color resolves a hex colour or a style token to a colour.
func (t Template) color(s string, def color.NRGBA) color.NRGBA {
	switch s {
	case "":
		return def
	case "bg":
		s = t.Background
	case "accent":
		s = t.Accent
	case "text":
		s = t.TextColor
	case "border":
		s = t.BorderCol
	}
	if c, err := ParseColor(s); err == nil {
		return c
	}
	return def
}

// AssetNames lists every image file the frame refers to.
func (t Template) AssetNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	add(t.BackgroundImage)
	add(t.Overlay)
	for _, l := range t.Layers {
		if l.Kind == LayerImage {
			add(l.Asset)
		}
	}
	return out
}

// AssetSource resolves a frame's image files.
type AssetSource = fs.FS
