package templates

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

func TestDefaultOfferIsMinimalAndTheWholeCatalogueIsStillThere(t *testing.T) {
	lib := Load(t.TempDir())
	// a new installation shows guests only the undecorated Plain frame for each photo count
	var offered []string
	counts := map[int]bool{}
	for _, f := range lib.Active() {
		offered = append(offered, f.ID)
		counts[f.Photos()] = true
		if f.Design != "plain" {
			t.Errorf("%s is decorated but offered by default", f.ID)
		}
	}
	if len(offered) != 5 {
		t.Fatalf("minimal default set expected (5 plain frames), got %v", offered)
	}
	for _, n := range []int{1, 2, 3, 4, 6} {
		if !counts[n] {
			t.Errorf("no default frame takes %d photos", n)
		}
	}
	if f := lib.Get(DefaultFrameID); f == nil || !f.Enabled || f.Photos() != 4 || f.Design != "plain" {
		t.Fatalf("the preselected default must be the Plain 4-photo strip: %+v", f)
	}
	// every design is still available to the operator (disabled until switched on)
	cats, names := map[string]bool{}, map[string]bool{}
	for _, f := range lib.All() {
		if !f.Legacy {
			cats[f.Category] = true
			names[f.Name] = true
		}
	}
	for _, c := range []string{CatClassic, CatPastel, CatRetro, CatSpecial, CatPlain} {
		if !cats[c] {
			t.Errorf("category %s is missing from the catalogue", c)
		}
	}
	for _, n := range []string{"White Minimal", "Black Minimal", "Clean Border", "Soft Gray", "Rose Pink", "Lavender", "Cream Beige", "Sky Blue",
		"Film Strip", "Vintage Camera", "Polaroid Inspired", "Monochrome Film", "Floral", "Birthday", "Couple", "Seasonal", "Plain"} {
		if !names[n] {
			t.Errorf("built-in frame %q is missing", n)
		}
	}
	// every layout × design exists (disabled ones are for the operator to enable)
	all := lib.All()
	byID := map[string]Template{}
	for _, f := range all {
		byID[f.ID] = f
	}
	want := map[string][2]int{"strip4": {600, 1800}, "strip3": {600, 1800}, "strip2": {600, 1800}, "twoup4": {1200, 1800}, "sheet6": {1200, 1800}, "sheet4": {1200, 1800}, "sheet2": {1200, 1800}, "p57": {1500, 2100}, "single": {1200, 1800}}
	for lid, px := range want {
		f, ok := byID["rose-pink."+lid]
		if !ok {
			t.Fatalf("rose-pink.%s missing", lid)
		}
		if w, h := f.PixelSize(); w != px[0] || h != px[1] || f.DPI != 300 {
			t.Errorf("%s is %dx%d@%d, want %dx%d@300", f.ID, w, h, f.DPI, px[0], px[1])
		}
	}
	if byID["rose-pink.twoup4"].Photos() != 4 || len(byID["rose-pink.twoup4"].Slots) != 8 {
		t.Error("two-up must repeat four photos into two strips")
	}
}

func TestEveryFrameValidatesAndEveryVariantRenders(t *testing.T) {
	for _, f := range Load(t.TempDir()).All() {
		if err := f.Validate(); err != nil {
			t.Fatalf("%s: %v", f.ID, err)
		}
		for _, v := range f.Variants {
			r, err := f.WithVariant(v.ID)
			if err != nil || r.Validate() != nil {
				t.Fatalf("%s/%s: %v", f.ID, v.ID, err)
			}
		}
	}
	// render one frame per design × variant at preview size
	for _, f := range Load(t.TempDir()).All() {
		if f.LayoutID != "strip4" {
			continue
		}
		for _, v := range append([]string{""}, variantIDs(f)...) {
			r, _ := f.WithVariant(v)
			if _, err := Preview(r, nil, RenderContext{}, 200); err != nil {
				t.Fatalf("%s/%s: %v", f.ID, v, err)
			}
		}
	}
}

func TestUnknownVariantIsAnErrorNotASubstitution(t *testing.T) {
	f := *Load(t.TempDir()).Get("rose-pink.strip4")
	if _, err := f.WithVariant("nope"); err == nil {
		t.Fatal("an unknown colour option must not fall back silently")
	}
	r, err := f.WithVariant("coral")
	if err != nil || r.Background != "#f9c4b4" || r.VariantID != "coral" || f.Background != "#f9d5dc" {
		t.Fatalf("variant must be applied to a copy: %v %+v", err, r.Style)
	}
}

func TestSlotGeometryRules(t *testing.T) {
	base := *Load(t.TempDir()).Get("plain.strip4")
	gap := base.Clone()
	gap.Slots[3].Photo = 5 // photos 0,1,2,5 — 3 and 4 missing
	if gap.Validate() == nil {
		t.Error("a gap in photo numbers must be rejected")
	}
	three := *Load(t.TempDir()).Get("plain.strip3")
	if three.Photos() != 3 || three.Validate() != nil {
		t.Error("a three-photo frame must be valid")
	}
	out := base.Clone()
	out.Slots[0].X = 1.9
	if out.Validate() == nil {
		t.Error("slot outside canvas")
	}
	many := base.Clone()
	for len(many.Slots) <= maxSlots {
		many.Slots = append(many.Slots, many.Slots[0])
	}
	if many.Validate() == nil {
		t.Error("too many slots")
	}
	for _, bad := range []string{"../x", "a b", "", "a/b"} {
		b := base.Clone()
		b.ID = bad
		if b.Validate() == nil {
			t.Errorf("id %q accepted", bad)
		}
	}
}

func TestLayerValidation(t *testing.T) {
	base := *Load(t.TempDir()).Get("plain.strip4")
	bad := []Layer{
		{Kind: "script"},
		{Kind: LayerText, SizeIn: 0.1, Text: strings.Repeat("x", 300)},
		{Kind: LayerText, SizeIn: 0, Text: "x"},
		{Kind: LayerText, SizeIn: 0.1, Font: "comic"},
		{Kind: LayerShape, Shape: "dragon"},
		{Kind: LayerImage, Asset: "../../etc/passwd.png"},
		{Kind: LayerImage, Asset: "logo.svg"},
		{Kind: LayerImage, Asset: "http://evil/x.png"},
		{Kind: LayerRect, Color: "red"},
		{Kind: LayerPattern, Shape: "dot", Size: 0.001, W: 1, H: 1},
		{Kind: LayerPattern, Shape: "dot", Size: 0.02, W: 20, H: 30},
	}
	for i, l := range bad {
		f := base.Clone()
		f.Layers = []Layer{l}
		if f.Validate() == nil {
			t.Errorf("bad layer %d accepted: %+v", i, l)
		}
	}
	f := base.Clone()
	f.Overlay = "x.svg"
	if f.Validate() == nil {
		t.Error("svg overlay accepted")
	}
}

func TestCaptionAndDateText(t *testing.T) {
	rc := RenderContext{Caption: "Mai & Nam", Date: time.Date(2026, 11, 22, 18, 5, 0, 0, time.UTC)}
	for in, want := range map[string]string{
		"{caption|date}": "Mai & Nam", "{date}": "2026.11.22", "hi {caption}!": "hi Mai & Nam!", "{caption|Us}": "Mai & Nam", "{time}": "18:05", "plain": "plain",
	} {
		if got := ExpandText(in, rc); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	if got := ExpandText("{caption|HAPPY BIRTHDAY}", RenderContext{}); got != "HAPPY BIRTHDAY" {
		t.Errorf("fallback literal: %q", got)
	}
	if got := ExpandText("{caption|date}", RenderContext{}); got != "" {
		t.Errorf("nothing to show: %q", got)
	}
	// the caption really reaches the pixels
	f := *Load(t.TempDir()).Get("white-minimal.strip4")
	a, _ := Preview(f, nil, RenderContext{}, 300)
	b, _ := Preview(f, nil, RenderContext{Caption: "HELLO"}, 300)
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("caption did not change the rendering")
	}
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestTransparentOverlayLayersOverPhotosAndKeepsTransparency(t *testing.T) {
	dir := t.TempDir()
	ov := image.NewNRGBA(image.Rect(0, 0, 200, 200)) // fully transparent except a half-transparent red band and an opaque blue dot
	for x := 0; x < 200; x++ {
		for y := 90; y < 110; y++ {
			ov.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 128})
		}
	}
	for x := 10; x < 30; x++ {
		for y := 10; y < 30; y++ {
			ov.SetNRGBA(x, y, color.NRGBA{0, 0, 255, 255})
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "ov.png"), pngBytes(ov), 0o644)
	tpl := Template{ID: "o", Geometry: Geometry{DPI: 100, WidthIn: 2, HeightIn: 2, Slots: []Slot{{X: 0, Y: 0, W: 2, H: 2}}}, Style: Style{Background: "#000000", Overlay: "ov.png"}}
	out, err := Compose(tpl, []image.Image{solid(100, 100, color.NRGBA{0, 255, 0, 255})}, os.DirFS(dir), RenderContext{})
	if err != nil {
		t.Fatal(err)
	}
	if c := out.NRGBAAt(100, 5); c.G < 250 || c.R > 3 || c.B > 3 {
		t.Fatalf("transparent area must show the photo, got %v", c)
	}
	if c := out.NRGBAAt(100, 100); c.R < 110 || c.R > 145 || c.G < 110 || c.G > 145 {
		t.Fatalf("50%% red over green must blend, got %v", c)
	}
	if c := out.NRGBAAt(20, 20); c.B < 250 || c.G > 5 {
		t.Fatalf("opaque overlay pixel must cover the photo, got %v", c)
	}
}

func TestOverlayIsDrawnAboveBordersAndTextAboveOverlay(t *testing.T) {
	dir := t.TempDir()
	ov := media.Solid(100, 100, color.NRGBA{255, 0, 0, 255})
	_ = os.WriteFile(filepath.Join(dir, "ov.png"), pngBytes(ov), 0o644)
	tpl := Template{ID: "o", Geometry: Geometry{DPI: 100, WidthIn: 2, HeightIn: 2, Slots: []Slot{{X: 0.2, Y: 0.2, W: 1.6, H: 1.6}}},
		Style: Style{Background: "#ffffff", Overlay: "ov.png", BorderIn: 0.1, BorderCol: "#00ff00",
			Layers: []Layer{
				{ID: "under", Kind: LayerRect, X: 0, Y: 0, W: 2, H: 2, Color: "#0000ff", Under: true},
				{ID: "over", Kind: LayerRect, X: 0.9, Y: 0.9, W: 0.2, H: 0.2, Color: "#ffff00"},
			}}}
	out, err := Compose(tpl, []image.Image{solid(10, 10, color.NRGBA{0, 0, 0, 255})}, os.DirFS(dir), RenderContext{})
	if err != nil {
		t.Fatal(err)
	}
	if c := out.NRGBAAt(100, 50); c.R != 255 || c.G != 0 {
		t.Fatalf("an opaque overlay hides under-layers, photos and borders: %v", c)
	}
	if c := out.NRGBAAt(100, 100); c.R != 255 || c.G != 255 || c.B != 0 {
		t.Fatalf("an over layer is drawn above the overlay: %v", c)
	}
}

func TestRoundedCornersAndBordersAreAntialiasedNotSquare(t *testing.T) {
	tpl := Template{ID: "r", Geometry: Geometry{DPI: 100, WidthIn: 2, HeightIn: 2, Slots: []Slot{{X: 0.5, Y: 0.5, W: 1, H: 1}}}, Style: Style{Background: "#ff0000", CornerIn: 0.3}}
	out, err := Compose(tpl, []image.Image{solid(50, 50, color.NRGBA{0, 0, 255, 255})}, nil, RenderContext{})
	if err != nil {
		t.Fatal(err)
	}
	if c := out.NRGBAAt(51, 51); c.R < 250 { // the very corner of the slot is rounded away: background shows
		t.Fatalf("corner pixel should be background, got %v", c)
	}
	if c := out.NRGBAAt(100, 100); c.B < 250 {
		t.Fatalf("centre should be the photo, got %v", c)
	}
}

func TestPreviewMatchesFinalRenderAtAnyDPI(t *testing.T) {
	// the preview (low DPI) and the 300-DPI output come from one renderer: downscaling the output must match the preview
	lib := Load(t.TempDir())
	for _, id := range []string{"rose-pink.strip4", "film-strip.twoup4", "vintage-camera.strip3", "birthday.sheet6", "polaroid.p57"} {
		f := *lib.Get(id)
		rc := RenderContext{Date: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), Caption: "x"}
		small, err := Preview(f, nil, rc, 200)
		if err != nil {
			t.Fatal(err)
		}
		big, err := Compose(f, SamplePhotos(f.Photos()), nil, rc)
		if err != nil {
			t.Fatal(err)
		}
		down := image.NewNRGBA(small.Bounds())
		scaleDown(down, big)
		if d := meanAbsDiff(small, down); d > 9 {
			t.Errorf("%s: preview differs from the final render (mean diff %.1f/255)", id, d)
		}
	}
}

func TestSnapshotIsImmutableAndSurvivesFrameDeletion(t *testing.T) {
	dir := t.TempDir()
	ov := media.Solid(10, 10, color.NRGBA{255, 0, 255, 128})
	_ = os.WriteFile(filepath.Join(dir, "ov.png"), pngBytes(ov), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "mine.json"), []byte(`{"id":"mine","name":"Mine","widthIn":2,"heightIn":2,"dpi":100,"background":"#123456","overlay":"ov.png","slots":[{"x":0.2,"y":0.2,"w":1.6,"h":1.6,"photo":0}],"variants":[{"id":"alt","name":"Alt","background":"#654321"}]}`), 0o644)
	lib := Load(dir)
	snapDir := filepath.Join(t.TempDir(), "frames", "001")
	snap, err := lib.TakeSnapshot(snapDir, "mine", "alt", "ignored: caption not enabled", time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Frame.Background != "#654321" || snap.Caption != "" {
		t.Fatalf("snapshot must hold the resolved variant: %+v", snap)
	}
	if _, err := lib.TakeSnapshot(snapDir, "mine", "", "", time.Now(), 0); err == nil {
		t.Fatal("an existing snapshot must never be overwritten")
	}
	// the operator deletes the frame and its artwork
	_ = os.Remove(filepath.Join(dir, "mine.json"))
	_ = os.Remove(filepath.Join(dir, "ov.png"))
	lib.Reload()
	if lib.Get("mine") != nil {
		t.Fatal("frame should be gone")
	}
	got, assets, err := ReadSnapshot(snapDir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Compose(got.Frame, []image.Image{solid(10, 10, color.NRGBA{0, 255, 0, 255})}, assets, got.Context())
	if err != nil {
		t.Fatalf("a session must still render after its frame was deleted: %v", err)
	}
	if c := out.NRGBAAt(5, 5); c.R < 170 || c.R > 186 || c.G < 28 || c.G > 38 { // overlay (50% magenta) over the variant background #654321
		t.Fatalf("snapshot overlay not applied: %v", c)
	}
	// a damaged snapshot is reported, not rendered wrongly
	_ = os.WriteFile(filepath.Join(snapDir, "frame.json"), []byte("{"), 0o644)
	if _, _, err := ReadSnapshot(snapDir); err == nil {
		t.Fatal("damaged snapshot accepted")
	}
}

func TestEnabledStatePersists(t *testing.T) {
	dir := t.TempDir()
	lib := Load(dir)
	if lib.Get("lavender.sheet6").Enabled {
		t.Fatal("non-primary layouts start disabled")
	}
	if err := lib.SetEnabled("lavender.sheet6", true); err != nil {
		t.Fatal(err)
	}
	if err := lib.SetEnabled("rose-pink.strip4", false); err != nil {
		t.Fatal(err)
	}
	if lib.SetEnabled("strip-2x6-4", true) == nil || lib.SetEnabled("nope", true) == nil {
		t.Fatal("legacy/unknown frames cannot be enabled")
	}
	again := Load(dir)
	if !again.Get("lavender.sheet6").Enabled || again.Get("rose-pink.strip4").Enabled {
		t.Fatal("enabled state not persisted")
	}
	for _, f := range again.Active() {
		if f.ID == "rose-pink.strip4" {
			t.Fatal("a disabled frame is still offered")
		}
	}
}

func TestDecodeImageLimits(t *testing.T) {
	if _, err := DecodeImage([]byte("<svg onload=alert(1)>")); err == nil {
		t.Error("svg/garbage accepted")
	}
	if _, err := DecodeImage(pngBytes(media.Solid(20, 20, color.NRGBA{1, 2, 3, 4}))); err != nil {
		t.Errorf("valid png rejected: %v", err)
	}
	// a PNG header that claims 20000x20000 must be refused before any pixel memory is allocated
	big := pngBytes(media.Solid(8, 8, color.NRGBA{}))
	big[16], big[17], big[18], big[19] = 0, 0, 0x4e, 0x20 // width 20000
	big[20], big[21], big[22], big[23] = 0, 0, 0x4e, 0x20 // height 20000
	binary.BigEndian.PutUint32(big[29:33], crc32.ChecksumIEEE(big[12:29]))
	if _, err := DecodeImage(big); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("oversized image accepted: %v", err)
	}
}

// helpers
func scaleDown(dst, src *image.NRGBA) {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			var r, g, b, n int
			x0, x1 := x*sw/dst.Bounds().Dx(), (x+1)*sw/dst.Bounds().Dx()
			y0, y1 := y*sh/dst.Bounds().Dy(), (y+1)*sh/dst.Bounds().Dy()
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.NRGBAAt(xx, yy)
					r, g, b, n = r+int(c.R), g+int(c.G), b+int(c.B), n+1
				}
			}
			if n > 0 {
				dst.SetNRGBA(x, y, color.NRGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255})
			}
		}
	}
}

func meanAbsDiff(a, b *image.NRGBA) float64 {
	if a.Bounds() != b.Bounds() {
		return 255
	}
	var sum, n float64
	for i := 0; i+3 < len(a.Pix); i += 4 {
		for k := 0; k < 3; k++ {
			d := float64(a.Pix[i+k]) - float64(b.Pix[i+k])
			if d < 0 {
				d = -d
			}
			sum += d
			n++
		}
	}
	return sum / n
}
