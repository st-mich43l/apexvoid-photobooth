package templates

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The capture guide in the Booth UI (frontend/src/features/booth/captureGeometry.ts) must show exactly the part of the
// photograph the compositor will use. This test pins the compositor's crops for representative frames and camera
// sizes in testdata/slotcrops.json; the frontend test reads the same file and requires its own calculation to match.
// Regenerate after an intentional compositor change with:  UPDATE_GOLDEN=1 go test ./internal/templates -run SlotCropGolden

type cropGoldenSlot struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	Photo  int     `json:"photo"`
	FocusX float64 `json:"focusX"`
	FocusY float64 `json:"focusY"`
}
type cropGoldenCase struct {
	ID       string           `json:"id"`
	DPI      int              `json:"dpi"`
	BorderIn float64          `json:"borderIn"`
	Slots    []cropGoldenSlot `json:"slots"`
	Sources  []cropGoldenSrc  `json:"sources"`
}
type cropGoldenSrc struct {
	W     int        `json:"w"`
	H     int        `json:"h"`
	Crops []SlotCrop `json:"crops"`
}

func cropGoldenTemplates() []Template {
	var out []Template
	seen := map[string]bool{}
	for _, t := range designCatalogue() {
		// one design on every layout, plus every design on the 4-photo strip (borders, corners)
		if (t.Design == "plain" && !seen[t.LayoutID]) || t.LayoutID == "strip4" {
			if t.Design == "plain" {
				seen[t.LayoutID] = true
			}
			out = append(out, t)
		}
	}
	sheet := Template{ID: "synthetic.sheet6-border-focus", Geometry: Geometry{WidthIn: 4, HeightIn: 6, DPI: 300, Slots: []Slot{
		{X: 0.2, Y: 0.2, W: 1.7, H: 1.7, Photo: 0, FocusX: 0.2, FocusY: 0.8}, {X: 2.1, Y: 0.2, W: 1.7, H: 1.7, Photo: 1, FocusX: 0.9},
		{X: 0.2, Y: 2.2, W: 3.6, H: 1.4, Photo: 2}, {X: 0.2, Y: 3.8, W: 1.7, H: 1.9, Photo: 3, FocusY: 0.3}, {X: 2.1, Y: 3.8, W: 1.7, H: 1.9, Photo: 4}, {X: 1.2, Y: 5.0, W: 1.0, H: 0.9, Photo: 5},
	}}, Style: Style{BorderIn: 0.08}}
	repeated := Template{ID: "synthetic.twoup-repeated", Geometry: Geometry{WidthIn: 4, HeightIn: 6, DPI: 300, Slots: []Slot{
		{X: 0.1, Y: 0.1, W: 0.9, H: 2.7, Photo: 0}, {X: 0.1, Y: 3.1, W: 0.9, H: 2.7, Photo: 1}, {X: 2.1, Y: 0.1, W: 0.9, H: 2.7, Photo: 0, FocusX: 0.7}, {X: 2.1, Y: 3.1, W: 0.9, H: 2.7, Photo: 1},
	}}}
	return append(out, sheet, repeated)
}

func buildCropGolden() []cropGoldenCase {
	sizes := [][2]int{{6000, 4000}, {4000, 3000}, {1920, 1080}, {1280, 720}, {4000, 6000}}
	var cases []cropGoldenCase
	for _, t := range cropGoldenTemplates() {
		c := cropGoldenCase{ID: t.ID, DPI: t.DPI, BorderIn: t.BorderIn}
		for _, s := range t.Slots {
			c.Slots = append(c.Slots, cropGoldenSlot{s.X, s.Y, s.W, s.H, s.Photo, s.FocusX, s.FocusY})
		}
		for _, sz := range sizes {
			c.Sources = append(c.Sources, cropGoldenSrc{W: sz[0], H: sz[1], Crops: t.SlotCrops(sz[0], sz[1])})
		}
		cases = append(cases, c)
	}
	return cases
}

func TestSlotCropGolden(t *testing.T) {
	path := filepath.Join("testdata", "slotcrops.json")
	got := buildCropGolden()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		b, _ := json.Marshal(got)
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want []cropGoldenCase
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	// compare through JSON so image.Rectangle round-trips identically
	gj, _ := json.Marshal(got)
	var back []cropGoldenCase
	_ = json.Unmarshal(gj, &back)
	if !reflect.DeepEqual(back, want) {
		t.Fatal("slot crops changed: the capture guide would no longer match the compositor. If intentional, regenerate with UPDATE_GOLDEN=1 and update captureGeometry.ts")
	}
}

// The crop must be what Compose really uses: a photo whose pixels encode their own coordinates is composed and the
// window must show exactly the predicted source region.
func TestSlotCropMatchesComposedPixels(t *testing.T) {
	tmpl := cropGoldenTemplates()[len(cropGoldenTemplates())-2] // synthetic sheet with border and off-centre focus
	src := image.NewNRGBA(image.Rect(0, 0, 600, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 600; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x / 3), G: uint8(y / 2), B: 40, A: 255})
		}
	}
	photos := make([]image.Image, tmpl.Photos())
	for i := range photos {
		photos[i] = src
	}
	canvas, err := compose(tmpl, photos, nil, RenderContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range tmpl.SlotCrops(600, 400) {
		// the window's centre pixel must come from (near) the centre of the predicted crop
		cx, cy := (sc.Window.Min.X+sc.Window.Max.X)/2, (sc.Window.Min.Y+sc.Window.Max.Y)/2
		got := canvas.NRGBAAt(cx, cy)
		wantX, wantY := (sc.Crop.Min.X+sc.Crop.Max.X)/2, (sc.Crop.Min.Y+sc.Crop.Max.Y)/2
		if dx, dy := int(got.R)-wantX/3, int(got.G)-wantY/2; dx < -2 || dx > 2 || dy < -2 || dy > 2 {
			t.Fatalf("slot %+v: window centre shows source (%d,%d), crop centre is (%d,%d)", sc, int(got.R)*3, int(got.G)*2, wantX, wantY)
		}
	}
}
