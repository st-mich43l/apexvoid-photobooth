package media

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x * 255 / w), uint8(y * 255 / h), 100, 255})
		}
	}
	b, err := EncodeJPEG(img, 90)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// withOrientation injects an EXIF APP1 segment carrying the orientation.
func withOrientation(jpegData []byte, o uint16) []byte {
	var tiff bytes.Buffer
	tiff.WriteString("MM")
	_ = binary.Write(&tiff, binary.BigEndian, uint16(42))
	_ = binary.Write(&tiff, binary.BigEndian, uint32(8))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(1))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(0x0112))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(3))
	_ = binary.Write(&tiff, binary.BigEndian, uint32(1))
	_ = binary.Write(&tiff, binary.BigEndian, o)
	_ = binary.Write(&tiff, binary.BigEndian, uint16(0))
	_ = binary.Write(&tiff, binary.BigEndian, uint32(0))
	seg := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	var out bytes.Buffer
	out.Write(jpegData[:2])
	out.Write([]byte{0xFF, 0xE1})
	_ = binary.Write(&out, binary.BigEndian, uint16(len(seg)+2))
	out.Write(seg)
	out.Write(jpegData[2:])
	return out.Bytes()
}

func TestOrientationAndDimensions(t *testing.T) {
	base := testJPEG(t, 120, 80)
	if Orientation(base) != 1 {
		t.Fatal("no EXIF should be orientation 1")
	}
	rot := withOrientation(base, 6)
	if Orientation(rot) != 6 {
		t.Fatalf("got %d", Orientation(rot))
	}
	w, h, err := Dimensions(rot)
	if err != nil || w != 80 || h != 120 {
		t.Fatalf("dimensions = %dx%d err=%v", w, h, err)
	}
	img, err := Decode(rot)
	if err != nil || img.Bounds().Dx() != 80 || img.Bounds().Dy() != 120 {
		t.Fatalf("decode did not orient: %v %v", img.Bounds(), err)
	}
}

func TestFitNeverUpscales(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 50))
	if g := Fit(src, 400, 400).Bounds(); g.Dx() != 100 {
		t.Fatalf("upscaled: %v", g)
	}
	if g := Fit(src, 50, 50).Bounds(); g.Dx() != 50 || g.Dy() != 25 {
		t.Fatalf("fit wrong: %v", g)
	}
}

func TestApplyEdits(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for i := range src.Pix {
		src.Pix[i] = 128
		if i%4 == 3 {
			src.Pix[i] = 255
		}
	}
	out, err := Apply(src, Edits{Rotate: 90, Crop: &Rect{0, 0, 0.5, 0.5}})
	if err != nil {
		t.Fatal(err)
	}
	if b := out.Bounds(); b.Dx() != 10 || b.Dy() != 20 {
		t.Fatalf("rotate+crop size %v", b)
	}
	bright, _ := Apply(src, Edits{Brightness: 0.2})
	if r, _, _, _ := bright.At(0, 0).RGBA(); r>>8 <= 128 {
		t.Fatal("brightness had no effect")
	}
	gray, _ := Apply(src, Edits{Saturation: -1})
	c := gray.At(1, 1).(color.NRGBA)
	if c.R != c.G || c.G != c.B {
		t.Fatal("saturation -1 must be grey")
	}
	if src.Pix[0] != 128 {
		t.Fatal("source mutated")
	}
	if _, err := Apply(src, Edits{Brightness: 2}); err == nil {
		t.Fatal("expected validation error")
	}
	if _, err := Apply(src, Edits{Crop: &Rect{0.8, 0, 0.5, 1}}); err == nil {
		t.Fatal("expected crop validation error")
	}
}

func TestDensity(t *testing.T) {
	b := SetJPEGDensity(testJPEG(t, 20, 20), 300)
	u, x, y, ok := JPEGDensity(b)
	if !ok || u != 1 || x != 300 || y != 300 {
		t.Fatalf("density %v %v %v %v", u, x, y, ok)
	}
	if _, err := Decode(b); err != nil {
		t.Fatalf("patched JPEG no longer decodes: %v", err)
	}
}
