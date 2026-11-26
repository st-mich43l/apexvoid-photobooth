package templates

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // asset decoders
	_ "image/png"
	"io"
	"io/fs"
	"math"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/vector"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

// RenderContext carries the per-session values text layers may show.
type RenderContext struct {
	Caption string    // the guest's short caption ("" = none)
	Date    time.Time // session date for {date} (zero = no date)
}

// Limits for imported artwork (also enforced at import time).
const (
	MaxAssetBytes  = 15 << 20
	MaxAssetPixels = 40_000_000
	MaxAssetSide   = 10_000
)

// Composition order (the same for every frame, preview and print):
//
//	canvas background → background artwork → layers marked "under" →
//	photo slots (crop, border, rounded corners) → transparent overlay →
//	remaining layers (decorations, images, text) in list order.
//
// Compose renders photos into t at its exact pixel size. assets resolves the
// frame's image files (may be nil when the frame has none).
func Compose(t Template, photos []image.Image, assets fs.FS, rc RenderContext) (*image.NRGBA, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return compose(t, photos, assets, rc)
}

func compose(t Template, photos []image.Image, assets fs.FS, rc RenderContext) (*image.NRGBA, error) {
	if len(photos) < t.Photos() {
		return nil, fmt.Errorf("frame %q needs %d photos, got %d", t.ID, t.Photos(), len(photos))
	}
	cw, ch := t.PixelSize()
	canvas := media.Solid(cw, ch, t.color(t.Background, color.NRGBA{255, 255, 255, 255}))
	dpi := float64(t.DPI)
	px := func(in float64) int { return int(math.Round(in * dpi)) }
	cache := map[string]image.Image{}
	load := func(name string) (image.Image, error) {
		if img, ok := cache[name]; ok {
			return img, nil
		}
		img, err := decodeAsset(assets, name)
		if err != nil {
			return nil, err
		}
		cache[name] = img
		return img, nil
	}

	if t.BackgroundImage != "" {
		img, err := load(t.BackgroundImage)
		if err != nil {
			return nil, fmt.Errorf("background image: %w", err)
		}
		cover(canvas, canvas.Bounds(), img, 0.5, 0.5)
	}
	layerPass := func(under bool) error {
		for _, l := range t.Layers {
			if l.Hidden || l.Under != under {
				continue
			}
			if err := drawLayer(canvas, t, l, px, load, rc); err != nil {
				return fmt.Errorf("layer %q: %w", layerName(l), err)
			}
		}
		return nil
	}
	if err := layerPass(true); err != nil {
		return nil, err
	}

	corner := px(t.CornerIn)
	for _, s := range t.Slots {
		r := image.Rect(px(s.X), px(s.Y), px(s.X+s.W), px(s.Y+s.H))
		inner := corner
		if t.BorderIn > 0 {
			bc := t.color(t.BorderCol, color.NRGBA{255, 255, 255, 255})
			b := px(t.BorderIn)
			if corner > 0 {
				fillRRect(canvas, r, corner, bc)
			} else {
				draw.Draw(canvas, r, image.NewUniform(bc), image.Point{}, draw.Src)
			}
			inner = max(0, corner-b)
		}
		r = t.photoWindow(s)
		if r.Empty() {
			continue
		}
		fx, fy := slotFocus(s)
		if inner > 0 {
			tmp := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
			cover(tmp, tmp.Bounds(), photos[s.Photo], fx, fy)
			mask := roundedMask(r.Dx(), r.Dy(), inner)
			draw.DrawMask(canvas, r, tmp, image.Point{}, mask, image.Point{}, draw.Over)
		} else {
			cover(canvas, r, photos[s.Photo], fx, fy)
		}
	}

	if t.Overlay != "" {
		ov, err := load(t.Overlay)
		if err != nil {
			return nil, fmt.Errorf("overlay: %w", err)
		}
		xdraw.CatmullRom.Scale(canvas, canvas.Bounds(), ov, ov.Bounds(), xdraw.Over, nil)
	}
	if err := layerPass(false); err != nil {
		return nil, err
	}
	return canvas, nil
}

func layerName(l Layer) string {
	if l.Name != "" {
		return l.Name
	}
	if l.ID != "" {
		return l.ID
	}
	return l.Kind
}

// cover scales src to fill r (cropping the excess around the focus point).
func cover(dst *image.NRGBA, r image.Rectangle, src image.Image, fx, fy float64) {
	xdraw.CatmullRom.Scale(dst, r, src, CoverCrop(r.Dx(), r.Dy(), src.Bounds(), fx, fy), xdraw.Src, nil)
}

// CoverCrop is the part of a source image (bounds sb) that fills a w×h window when the image is scaled to cover it
// and the excess is cut around the focus point (fx, fy in 0..1; 0.5 is the centre). It is the single definition of
// the crop used by the compositor, and the one the capture guide in the UI must reproduce.
func CoverCrop(w, h int, sb image.Rectangle, fx, fy float64) image.Rectangle {
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	scale := math.Max(float64(w)/sw, float64(h)/sh)
	cw, chh := float64(w)/scale, float64(h)/scale // crop size in source pixels
	x0 := float64(sb.Min.X) + (sw-cw)*fx
	y0 := float64(sb.Min.Y) + (sh-chh)*fy
	return image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x0+cw)), int(math.Round(y0+chh))).Intersect(sb)
}

// slotFocus is the crop anchor of a slot: a zero coordinate means the default centre.
func slotFocus(s Slot) (fx, fy float64) {
	fx, fy = s.FocusX, s.FocusY
	if fx == 0 {
		fx = 0.5
	}
	if fy == 0 {
		fy = 0.5
	}
	return fx, fy
}

// photoWindow is the pixel rectangle a slot's photograph is drawn into: the slot rounded to pixels, less the frame's
// photo border.
func (t Template) photoWindow(s Slot) image.Rectangle {
	px := func(in float64) int { return int(math.Round(in * float64(t.DPI))) }
	r := image.Rect(px(s.X), px(s.Y), px(s.X+s.W), px(s.Y+s.H))
	if t.BorderIn > 0 {
		r = r.Inset(px(t.BorderIn))
	}
	return r
}

// SlotCrop says, for one photo window of a frame, where it is on the canvas and which part of a srcW×srcH
// photograph (after EXIF orientation) fills it.
type SlotCrop struct {
	Photo  int             `json:"photo"`  // index into the session's photos the compositor reads
	Window image.Rectangle `json:"window"` // canvas pixels, after the border inset
	Crop   image.Rectangle `json:"crop"`   // source pixels
}

// SlotCrops lists the crop of every photo window for a photograph of the given size. Candidate photographs are
// interchangeable, so this does not depend on which capture goes where.
func (t Template) SlotCrops(srcW, srcH int) []SlotCrop {
	src := image.Rect(0, 0, srcW, srcH)
	out := make([]SlotCrop, 0, len(t.Slots))
	for _, s := range t.Slots {
		r := t.photoWindow(s)
		if r.Empty() {
			continue
		}
		fx, fy := slotFocus(s)
		out = append(out, SlotCrop{Photo: s.Photo, Window: r, Crop: CoverCrop(r.Dx(), r.Dy(), src, fx, fy)})
	}
	return out
}

// decodeAsset reads and decodes an image from the frame's assets with size,
// dimension and memory limits (a decompression bomb is rejected from its header).
func decodeAsset(assets fs.FS, name string) (image.Image, error) {
	if assets == nil {
		return nil, fmt.Errorf("image %q is not available", name)
	}
	if !fs.ValidPath(name) || strings.Contains(name, "/") {
		return nil, fmt.Errorf("invalid image name %q", name)
	}
	f, err := assets.Open(name)
	if err != nil {
		return nil, fmt.Errorf("image %q is missing", name)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxAssetBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxAssetBytes {
		return nil, fmt.Errorf("image %q is larger than %d MB", name, MaxAssetBytes>>20)
	}
	return DecodeImage(data)
}

// DecodeImage decodes PNG or JPEG bytes after checking the header against the
// size limits. It does not trust the file name or MIME type.
func DecodeImage(data []byte) (image.Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("the file is not a valid PNG or JPEG image")
	}
	if format != "png" && format != "jpeg" {
		return nil, errors.New("only PNG and JPEG images are supported")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxAssetSide || cfg.Height > MaxAssetSide || cfg.Width*cfg.Height > MaxAssetPixels {
		return nil, fmt.Errorf("the image is too large (%d×%d px; limit %d MP)", cfg.Width, cfg.Height, MaxAssetPixels/1_000_000)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("the image file is damaged")
	}
	return img, nil
}

// ---- layers ----

func drawLayer(canvas *image.NRGBA, t Template, l Layer, px func(float64) int, load func(string) (image.Image, error), rc RenderContext) error {
	rect := image.Rect(px(l.X), px(l.Y), px(l.X+l.W), px(l.Y+l.H))
	col := t.color(l.Color, t.color("accent", color.NRGBA{0, 0, 0, 255}))
	if l.Color == "" && l.Kind == LayerText {
		col = t.color("text", color.NRGBA{20, 20, 20, 255})
	}
	op := l.Opacity
	if op == 0 {
		op = 1
	}
	col.A = uint8(math.Round(255 * op))

	switch l.Kind {
	case LayerRect:
		stroke := px(l.StrokeIn)
		rad := px(l.RadiusIn)
		fill(canvas, rect, col, func(z *vector.Rasterizer, ox, oy float32) {
			rrectPath(z, ox, oy, float32(rect.Dx()), float32(rect.Dy()), float32(rad), false)
			if stroke > 0 {
				s := float32(stroke)
				rrectPath(z, ox+s, oy+s, float32(rect.Dx())-2*s, float32(rect.Dy())-2*s, max(0, float32(rad)-s), true)
			}
		})
	case LayerEllipse:
		stroke := px(l.StrokeIn)
		fill(canvas, rect, col, func(z *vector.Rasterizer, ox, oy float32) {
			ellipsePath(z, ox, oy, float32(rect.Dx()), float32(rect.Dy()), false)
			if stroke > 0 {
				s := float32(stroke)
				ellipsePath(z, ox+s, oy+s, float32(rect.Dx())-2*s, float32(rect.Dy())-2*s, true)
			}
		})
	case LayerShape:
		fill(canvas, rect, col, func(z *vector.Rasterizer, ox, oy float32) {
			shapePath(z, l.Shape, ox, oy, float32(rect.Dx()), float32(rect.Dy()))
		})
	case LayerPattern:
		drawPattern(canvas, t, l, px, col)
	case LayerText:
		drawText(canvas, t, l, rect, px, col, rc)
	case LayerImage:
		img, err := load(l.Asset)
		if err != nil {
			return err
		}
		drawImageLayer(canvas, rect, img, l.Fit == "cover", col.A)
	}
	return nil
}

func drawImageLayer(canvas *image.NRGBA, rect image.Rectangle, img image.Image, coverFit bool, alpha uint8) {
	if rect.Empty() {
		return
	}
	dst := rect
	if !coverFit {
		sb := img.Bounds()
		s := math.Min(float64(rect.Dx())/float64(sb.Dx()), float64(rect.Dy())/float64(sb.Dy()))
		w, h := int(math.Round(float64(sb.Dx())*s)), int(math.Round(float64(sb.Dy())*s))
		x, y := rect.Min.X+(rect.Dx()-w)/2, rect.Min.Y+(rect.Dy()-h)/2
		dst = image.Rect(x, y, x+w, y+h)
	}
	tmp := image.NewNRGBA(image.Rect(0, 0, dst.Dx(), dst.Dy()))
	if coverFit {
		cover(tmp, tmp.Bounds(), img, 0.5, 0.5)
	} else {
		xdraw.CatmullRom.Scale(tmp, tmp.Bounds(), img, img.Bounds(), xdraw.Src, nil)
	}
	clip := dst.Intersect(canvas.Bounds())
	if clip.Empty() {
		return
	}
	draw.DrawMask(canvas, clip, tmp, clip.Min.Sub(dst.Min), image.NewUniform(color.Alpha{alpha}), image.Point{}, draw.Over)
}

// ---- vector helpers ----

// fill rasterises the path built by build (in coordinates local to area) and
// composites it over dst in col, clipped to dst.
func fill(dst *image.NRGBA, area image.Rectangle, col color.NRGBA, build func(z *vector.Rasterizer, ox, oy float32)) {
	clip := area.Intersect(dst.Bounds())
	if clip.Empty() || col.A == 0 {
		return
	}
	z := vector.NewRasterizer(clip.Dx(), clip.Dy())
	build(z, float32(area.Min.X-clip.Min.X), float32(area.Min.Y-clip.Min.Y))
	z.Draw(dst, clip, image.NewUniform(col), image.Point{})
}

const kappa = 0.5522847498

// rrectPath adds a (rounded) rectangle; reverse flips the winding so it cuts a hole in a previous path.
func rrectPath(z *vector.Rasterizer, x, y, w, h, r float32, reverse bool) {
	if w <= 0 || h <= 0 {
		return
	}
	r = float32(math.Min(float64(r), math.Min(float64(w), float64(h))/2))
	k := r * kappa
	if r <= 0 {
		if !reverse {
			z.MoveTo(x, y)
			z.LineTo(x+w, y)
			z.LineTo(x+w, y+h)
			z.LineTo(x, y+h)
		} else {
			z.MoveTo(x, y)
			z.LineTo(x, y+h)
			z.LineTo(x+w, y+h)
			z.LineTo(x+w, y)
		}
		z.ClosePath()
		return
	}
	if !reverse {
		z.MoveTo(x+r, y)
		z.LineTo(x+w-r, y)
		z.CubeTo(x+w-r+k, y, x+w, y+r-k, x+w, y+r)
		z.LineTo(x+w, y+h-r)
		z.CubeTo(x+w, y+h-r+k, x+w-r+k, y+h, x+w-r, y+h)
		z.LineTo(x+r, y+h)
		z.CubeTo(x+r-k, y+h, x, y+h-r+k, x, y+h-r)
		z.LineTo(x, y+r)
		z.CubeTo(x, y+r-k, x+r-k, y, x+r, y)
	} else {
		z.MoveTo(x+r, y)
		z.CubeTo(x+r-k, y, x, y+r-k, x, y+r)
		z.LineTo(x, y+h-r)
		z.CubeTo(x, y+h-r+k, x+r-k, y+h, x+r, y+h)
		z.LineTo(x+w-r, y+h)
		z.CubeTo(x+w-r+k, y+h, x+w, y+h-r+k, x+w, y+h-r)
		z.LineTo(x+w, y+r)
		z.CubeTo(x+w, y+r-k, x+w-r+k, y, x+w-r, y)
	}
	z.ClosePath()
}

func ellipsePath(z *vector.Rasterizer, x, y, w, h float32, reverse bool) {
	if w <= 0 || h <= 0 {
		return
	}
	rx, ry := w/2, h/2
	cx, cy := x+rx, y+ry
	kx, ky := rx*kappa, ry*kappa
	if !reverse {
		z.MoveTo(cx+rx, cy)
		z.CubeTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry)
		z.CubeTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy)
		z.CubeTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry)
		z.CubeTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy)
	} else {
		z.MoveTo(cx+rx, cy)
		z.CubeTo(cx+rx, cy-ky, cx+kx, cy-ry, cx, cy-ry)
		z.CubeTo(cx-kx, cy-ry, cx-rx, cy-ky, cx-rx, cy)
		z.CubeTo(cx-rx, cy+ky, cx-kx, cy+ry, cx, cy+ry)
		z.CubeTo(cx+kx, cy+ry, cx+rx, cy+ky, cx+rx, cy)
	}
	z.ClosePath()
}

func poly(z *vector.Rasterizer, ox, oy, w, h float32, pts [][2]float32) {
	for i, p := range pts {
		x, y := ox+p[0]*w, oy+p[1]*h
		if i == 0 {
			z.MoveTo(x, y)
		} else {
			z.LineTo(x, y)
		}
	}
	z.ClosePath()
}

// shapePath adds a named shape filling the box (ox,oy,w,h).
func shapePath(z *vector.Rasterizer, shape string, ox, oy, w, h float32) {
	p := func(x, y float32) (float32, float32) { return ox + x*w, oy + y*h }
	switch shape {
	case "dot":
		ellipsePath(z, ox, oy, w, h, false)
	case "square":
		rrectPath(z, ox, oy, w, h, 0, false)
	case "sprocket":
		rrectPath(z, ox, oy, w, h, min(w, h)*0.28, false)
	case "diamond":
		poly(z, ox, oy, w, h, [][2]float32{{0.5, 0}, {1, 0.5}, {0.5, 1}, {0, 0.5}})
	case "triangle":
		poly(z, ox, oy, w, h, [][2]float32{{0.5, 0}, {1, 1}, {0, 1}})
	case "star":
		var pts [][2]float32
		for i := 0; i < 10; i++ {
			a := -math.Pi/2 + float64(i)*math.Pi/5
			r := 0.5
			if i%2 == 1 {
				r = 0.21
			}
			pts = append(pts, [2]float32{0.5 + float32(r*math.Cos(a)), 0.53 + float32(r*math.Sin(a))})
		}
		poly(z, ox, oy, w, h, pts)
	case "sparkle": // four-pointed star
		poly(z, ox, oy, w, h, [][2]float32{{0.5, 0}, {0.6, 0.4}, {1, 0.5}, {0.6, 0.6}, {0.5, 1}, {0.4, 0.6}, {0, 0.5}, {0.4, 0.4}})
	case "heart":
		mv := func(x, y float32) { a, b := p(x, y); z.MoveTo(a, b) }
		cb := func(x1, y1, x2, y2, x, y float32) {
			a1, b1 := p(x1, y1)
			a2, b2 := p(x2, y2)
			a, b := p(x, y)
			z.CubeTo(a1, b1, a2, b2, a, b)
		}
		mv(0.5, 0.95)
		cb(0.18, 0.7, 0, 0.5, 0, 0.28)
		cb(0, 0.1, 0.14, 0, 0.27, 0)
		cb(0.38, 0, 0.46, 0.06, 0.5, 0.18)
		cb(0.54, 0.06, 0.62, 0, 0.73, 0)
		cb(0.86, 0, 1, 0.1, 1, 0.28)
		cb(1, 0.5, 0.82, 0.7, 0.5, 0.95)
		z.ClosePath()
	case "flower": // five petals around a centre
		for i := 0; i < 5; i++ {
			a := -math.Pi/2 + float64(i)*2*math.Pi/5
			cx, cy := 0.5+0.27*math.Cos(a), 0.5+0.27*math.Sin(a)
			ellipsePath(z, ox+float32(cx-0.23)*w, oy+float32(cy-0.23)*h, 0.46*w, 0.46*h, false)
		}
		ellipsePath(z, ox+0.3*w, oy+0.3*h, 0.4*w, 0.4*h, false)
	case "cloud":
		ellipsePath(z, ox+0.0*w, oy+0.4*h, 0.4*w, 0.6*h, false)
		ellipsePath(z, ox+0.2*w, oy+0.1*h, 0.45*w, 0.75*h, false)
		ellipsePath(z, ox+0.5*w, oy+0.25*h, 0.4*w, 0.6*h, false)
		ellipsePath(z, ox+0.6*w, oy+0.4*h, 0.4*w, 0.6*h, false)
		rrectPath(z, ox+0.18*w, oy+0.55*h, 0.64*w, 0.45*h, 0.2*h, false)
	}
}

// ---- patterns ----

func drawPattern(canvas *image.NRGBA, t Template, l Layer, px func(float64) int, base color.NRGBA) {
	w, h := l.Size, l.ElemH
	if h == 0 {
		h = w
	}
	if w <= 0 || l.W <= 0 || l.H <= 0 {
		return
	}
	cols := []color.NRGBA{base}
	if len(l.Colors) > 0 {
		cols = cols[:0]
		for _, c := range l.Colors {
			cc := t.color(c, base)
			cc.A = base.A
			cols = append(cols, cc)
		}
	}
	hf := fnv.New32a()
	_, _ = hf.Write([]byte(l.ID + l.Shape))
	rng := hf.Sum32() | 1
	next := func() float64 { // xorshift: deterministic, so every render is identical
		rng ^= rng << 13
		rng ^= rng >> 17
		rng ^= rng << 5
		return float64(rng%10000) / 10000
	}
	stepX, stepY := w+l.Gap, h+l.Gap
	i := 0
	for y := 0.0; y+h <= l.H+1e-9; y += stepY {
		for x := 0.0; x+w <= l.W+1e-9; x += stepX {
			ex, ey, ew, eh := l.X+x, l.Y+y, w, h
			if l.Spread {
				if next() < 0.32 {
					i++
					continue
				}
				k := 0.6 + 0.6*next()
				ew, eh = w*k, h*k
				ex += (next() - 0.5) * (stepX - ew)
				ey += (next() - 0.5) * (stepY - eh)
			}
			c := cols[i%len(cols)]
			i++
			r := image.Rect(px(ex), px(ey), px(ex+ew), px(ey+eh))
			shape := l.Shape
			fill(canvas, r, c, func(z *vector.Rasterizer, ox, oy float32) {
				shapePath(z, shape, ox, oy, float32(r.Dx()), float32(r.Dy()))
			})
		}
	}
}

// ---- rounded photo corners ----

func fillRRect(dst *image.NRGBA, r image.Rectangle, rad int, col color.NRGBA) {
	fill(dst, r, col, func(z *vector.Rasterizer, ox, oy float32) {
		rrectPath(z, ox, oy, float32(r.Dx()), float32(r.Dy()), float32(rad), false)
	})
}

func roundedMask(w, h, rad int) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	z := vector.NewRasterizer(w, h)
	rrectPath(z, 0, 0, float32(w), float32(h), float32(rad), false)
	z.DrawOp = draw.Src
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}
