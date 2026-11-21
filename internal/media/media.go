// Package media is pure image processing: EXIF orientation, thumbnails,
// non-destructive edit rendering and JPEG DPI metadata. It never touches the
// library on disk, so originals cannot be modified from here.
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"math"

	xdraw "golang.org/x/image/draw"
)

// Orientation reads the EXIF orientation (1-8) of a JPEG; 1 if absent.
func Orientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan / EOI
			return 1
		}
		l := int(binary.BigEndian.Uint16(data[i+2:]))
		if l < 2 || i+2+l > len(data) {
			return 1
		}
		if marker == 0xE1 && l >= 14 && string(data[i+4:i+10]) == "Exif\x00\x00" {
			return exifOrientation(data[i+10 : i+2+l])
		}
		i += 2 + l
	}
	return 1
}

func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// Decode decodes an image and applies its EXIF orientation.
func Decode(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return orient(img, Orientation(data)), nil
}

// Dimensions returns the displayed (orientation-corrected) size without
// decoding pixels.
func Dimensions(data []byte) (w, h int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	if o := Orientation(data); o >= 5 {
		return cfg.Height, cfg.Width, nil
	}
	return cfg.Width, cfg.Height, nil
}

func orient(src image.Image, o int) image.Image {
	switch o {
	case 2:
		return flipH(src)
	case 3:
		return Rotate(src, 180)
	case 4:
		return flipH(Rotate(src, 180))
	case 5:
		return flipH(Rotate(src, 90))
	case 6:
		return Rotate(src, 90)
	case 7:
		return flipH(Rotate(src, 270))
	case 8:
		return Rotate(src, 270)
	}
	return src
}

func flipH(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(b.Dx()-1-x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// Rotate rotates clockwise by 0/90/180/270 degrees.
func Rotate(src image.Image, deg int) image.Image {
	deg = ((deg % 360) + 360) % 360
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	switch deg {
	case 90:
		dst := image.NewNRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 180:
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 270:
		dst := image.NewNRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	}
	return src
}

// Fit scales src down so it fits within maxW x maxH, keeping aspect ratio.
// It never upscales.
func Fit(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return src
	}
	s := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	nw, nh := int(math.Max(1, math.Round(float64(w)*s))), int(math.Max(1, math.Round(float64(h)*s)))
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}

// EncodeJPEG encodes at the given quality.
func EncodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Rect is a normalised (0..1) crop rectangle in the rotated image.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Edits is the non-destructive edit recipe stored next to a media record.
type Edits struct {
	Rotate     int     `json:"rotate"`     // 0, 90, 180, 270 clockwise
	Crop       *Rect   `json:"crop"`       // nil = no crop
	Brightness float64 `json:"brightness"` // -1..1, 0 neutral
	Contrast   float64 `json:"contrast"`   // -1..1
	Saturation float64 `json:"saturation"` // -1..1
}

// Validate rejects out-of-range recipes.
func (e Edits) Validate() error {
	if e.Rotate%90 != 0 {
		return errors.New("rotate must be a multiple of 90")
	}
	for name, v := range map[string]float64{"brightness": e.Brightness, "contrast": e.Contrast, "saturation": e.Saturation} {
		if v < -1 || v > 1 || math.IsNaN(v) {
			return fmt.Errorf("%s must be within -1..1", name)
		}
	}
	if c := e.Crop; c != nil {
		if c.X < 0 || c.Y < 0 || c.W <= 0 || c.H <= 0 || c.X+c.W > 1.0001 || c.Y+c.H > 1.0001 {
			return errors.New("crop must be a rectangle inside the image (0..1)")
		}
	}
	return nil
}

// IsZero reports whether the recipe changes nothing.
func (e Edits) IsZero() bool {
	return e.Rotate%360 == 0 && e.Crop == nil && e.Brightness == 0 && e.Contrast == 0 && e.Saturation == 0
}

// Apply renders the recipe over src (src is not modified).
func Apply(src image.Image, e Edits) (image.Image, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	img := Rotate(src, e.Rotate)
	if c := e.Crop; c != nil {
		b := img.Bounds()
		r := image.Rect(
			b.Min.X+int(math.Round(c.X*float64(b.Dx()))), b.Min.Y+int(math.Round(c.Y*float64(b.Dy()))),
			b.Min.X+int(math.Round((c.X+c.W)*float64(b.Dx()))), b.Min.Y+int(math.Round((c.Y+c.H)*float64(b.Dy()))),
		).Intersect(b)
		if r.Empty() {
			return nil, errors.New("crop is empty")
		}
		dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.Draw(dst, dst.Bounds(), img, r.Min, draw.Src)
		img = dst
	}
	if e.Brightness == 0 && e.Contrast == 0 && e.Saturation == 0 {
		return img, nil
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	cf := 1 + e.Contrast
	sf := 1 + e.Saturation
	adj := func(v float64) uint8 {
		v = (v-127.5)*cf + 127.5 + e.Brightness*255
		return uint8(math.Max(0, math.Min(255, math.Round(v))))
	}
	for i := 0; i < len(dst.Pix); i += 4 {
		r, g, bl := float64(dst.Pix[i]), float64(dst.Pix[i+1]), float64(dst.Pix[i+2])
		y := 0.299*r + 0.587*g + 0.114*bl
		r, g, bl = y+(r-y)*sf, y+(g-y)*sf, y+(bl-y)*sf
		dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2] = adj(r), adj(g), adj(bl)
	}
	return dst, nil
}

// SetJPEGDensity writes dpi into a JFIF APP0 segment, inserting one when the
// encoder (Go's does not emit JFIF) left none, so printers and layout
// software see the intended physical size.
func SetJPEGDensity(data []byte, dpi int) []byte {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return data
	}
	if _, _, _, ok := JPEGDensity(data); !ok {
		app0 := []byte{0xFF, 0xE0, 0, 16, 'J', 'F', 'I', 'F', 0, 1, 1, 1, 0, 1, 0, 1, 0, 0}
		data = append(append(append([]byte(nil), data[:2]...), app0...), data[2:]...)
	} else {
		data = append([]byte(nil), data...)
	}
	data[13] = 1 // dots per inch
	binary.BigEndian.PutUint16(data[14:], uint16(dpi))
	binary.BigEndian.PutUint16(data[16:], uint16(dpi))
	return data
}

// JPEGDensity reads the JFIF density (units, x, y); ok=false if absent.
func JPEGDensity(data []byte) (units, x, y int, ok bool) {
	if len(data) > 18 && data[2] == 0xFF && data[3] == 0xE0 && string(data[6:11]) == "JFIF\x00" {
		return int(data[13]), int(binary.BigEndian.Uint16(data[14:])), int(binary.BigEndian.Uint16(data[16:])), true
	}
	return 0, 0, 0, false
}

// Solid returns a uniform image (used for backgrounds).
func Solid(w, h int, c color.Color) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return dst
}
