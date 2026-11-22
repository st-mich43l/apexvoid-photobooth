package app

import (
	"encoding/base64"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

// PNGFrameImport is the result of turning a picture into a frame draft.
type PNGFrameImport struct {
	Draft   templates.Template  `json:"draft"` // not saved yet: the builder opens on it
	Asset   templates.AssetInfo `json:"asset"`
	Windows int                 `json:"windows"` // transparent windows found (0 in "background" mode)
	Mode    string              `json:"mode"`    // "windows": overlay + slots under its windows · "background": picture under the photos
	Notes   []string            `json:"notes"`
}

// ImportPNGFrame is the fast way to use a ready-made frame picture. The picture is validated and
// copied like any builder import; then:
//
//   - a PNG with transparent windows becomes the overlay and one photo slot is placed under each
//     window (reading order, up to 12), on a canvas sized from the picture (600×1800 → 2×6 strip);
//   - a picture without usable windows (opaque, or no clear rectangles) becomes the background and
//     the photo slots come from the standard layout of that size — the operator moves them.
//
// Nothing is saved: the draft opens in the Frame Builder, ready to name and Save.
func (s *Service) ImportPNGFrame(dataURL, name string) (PNGFrameImport, error) {
	if err := s.guardWrite(); err != nil {
		return PNGFrameImport{}, err
	}
	info, err := s.ImportFrameImage(dataURL)
	if err != nil {
		return PNGFrameImport{}, err
	}
	b64 := dataURL
	if i := strings.Index(b64, ","); i >= 0 && strings.HasPrefix(b64, "data:") {
		b64 = b64[i+1:]
	}
	raw, _ := base64.StdEncoding.DecodeString(b64)
	img, err := templates.DecodeImage(raw)
	if err != nil {
		return PNGFrameImport{}, newUserErr(err.Error(), "")
	}
	name = strings.TrimSpace(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	if name == "" || name == "." {
		name = "Imported frame"
	}
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	wIn, hIn, dpi, standard := templates.GuessCanvas(info.Width, info.Height)
	res := PNGFrameImport{Asset: info}
	if standard {
		res.Notes = append(res.Notes, fmt.Sprintf("The picture is %d×%d px: used as a %g×%g in frame at %d dpi.", info.Width, info.Height, wIn, hIn, dpi))
	} else {
		res.Notes = append(res.Notes, fmt.Sprintf("The picture is %d×%d px, not a standard print size: used as %g×%g in at %d dpi. Check the size before printing.", info.Width, info.Height, wIn, hIn, dpi))
	}
	d := templates.Template{
		ID: s.tpl.UniqueID(name), Name: name, Kind: "custom", Category: templates.CatCustom, Enabled: true,
		Geometry: templates.Geometry{WidthIn: wIn, HeightIn: hIn, DPI: dpi},
		Style:    templates.Style{Background: "#ffffff"},
	}
	var wins []templates.PxRect
	if info.HasAlpha {
		wins = templates.DetectWindows(img)
	}
	if n := len(wins); n >= 1 && n <= 12 {
		res.Mode, res.Windows = "windows", n
		d.Slots = templates.SlotsFromWindows(wins, info.Width, info.Height, wIn, hIn)
		d.Overlay = info.Name
		res.Notes = append(res.Notes, fmt.Sprintf("Found %d transparent window%s: a photo slot was placed under each, numbered from the top left. Check them in the builder.", n, plural(n)))
		if math.Abs(float64(info.Width)/float64(info.Height)-wIn/hIn)/(wIn/hIn) > 0.02 {
			res.Notes = append(res.Notes, "The proportions differ a little from the print size, so the picture is stretched slightly.")
		}
	} else {
		res.Mode = "background"
		d.Slots = fallbackSlots(wIn, hIn)
		d.BackgroundImage = info.Name
		switch {
		case !info.HasAlpha:
			res.Notes = append(res.Notes, "The picture has no transparency, so it is used as the BACKGROUND and the photos are placed on top of it. For windows that show the photos, use a PNG with transparent areas.")
		case len(wins) > 12:
			res.Notes = append(res.Notes, fmt.Sprintf("Found %d transparent areas (at most 12 photos are supported), so the picture is used as the background with the standard photo layout.", len(wins)))
		default:
			res.Notes = append(res.Notes, "No clear transparent windows were found, so the picture is used as the background with the standard photo layout. Move the photo slots where you want them.")
		}
	}
	if info.Width*10 < int(wIn*float64(dpi))*9 || info.Height*10 < int(hIn*float64(dpi))*9 {
		res.Notes = append(res.Notes, fmt.Sprintf("It is smaller than %d×%d px (the print size at %d dpi) and may look soft.", int(math.Round(wIn*float64(dpi))), int(math.Round(hIn*float64(dpi))), dpi))
	}
	if err := d.Validate(); err != nil {
		return PNGFrameImport{}, newUserErr("This picture cannot be used as a frame: "+err.Error(), "")
	}
	if _, err := s.PreviewDraft(d, "", "", 120); err != nil { // proves it renders before the builder opens
		return PNGFrameImport{}, err
	}
	res.Draft = d
	s.log.Info("png frame imported", "mode", res.Mode, "windows", res.Windows, "px", fmt.Sprintf("%dx%d", info.Width, info.Height))
	return res, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// fallbackSlots places photos from the standard layout of that canvas (strip, sheet, 5×7) or one
// large slot inside a margin for any other size.
func fallbackSlots(wIn, hIn float64) []templates.Slot {
	short, long := math.Min(wIn, hIn), math.Max(wIn, hIn)
	id := ""
	switch {
	case near(short, 2) && near(long, 6) && wIn < hIn:
		id = "plain.strip4"
	case near(short, 4) && near(long, 6) && wIn < hIn:
		id = "plain.sheet4"
	case near(short, 5) && near(long, 7) && wIn < hIn:
		id = "plain.p57"
	}
	if id != "" {
		for _, t := range templates.Builtin() {
			if t.ID == id {
				return append([]templates.Slot(nil), t.Slots...)
			}
		}
	}
	m := math.Min(wIn, hIn) * 0.08
	return []templates.Slot{{X: round3(m), Y: round3(m), W: round3(wIn - 2*m), H: round3(hIn - 2*m)}}
}

func near(a, b float64) bool   { return math.Abs(a-b)/b < 0.03 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
