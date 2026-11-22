package templates

import (
	"fmt"
	"sort"
)

// The built-in frames are generated: every design (colours, borders, original
// vector decorations) is applied to every layout. All artwork is drawn by the
// renderer from shapes and the embedded Go fonts, so nothing here is third-party.

type layoutSpec struct {
	ID    string
	Label string
	Kind  string
	W, H  float64
	Slots []Slot
	// derived
	cols    [][2]float64 // x extent of each strip/column
	footers []rect       // free area under each column
}

type rect struct{ x, y, w, h float64 }

func (r rect) ok(minH float64) bool { return r.h >= minH && r.w > 0.3 }

func newLayout(id, label, kind string, w, h float64, slots []Slot) *layoutSpec {
	l := &layoutSpec{ID: id, Label: label, Kind: kind, W: w, H: h, Slots: slots}
	// columns: slots whose x ranges are closer than 0.25 in belong together
	type ext struct{ x0, x1, bottom float64 }
	var cs []ext
	sorted := append([]Slot(nil), slots...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].X < sorted[j].X })
	for _, s := range sorted {
		placed := false
		for i := range cs {
			if s.X < cs[i].x1+0.25 {
				cs[i].x1 = max(cs[i].x1, s.X+s.W)
				cs[i].bottom = max(cs[i].bottom, s.Y+s.H)
				placed = true
				break
			}
		}
		if !placed {
			cs = append(cs, ext{s.X, s.X + s.W, s.Y + s.H})
		}
	}
	for _, c := range cs {
		l.cols = append(l.cols, [2]float64{c.x0, c.x1})
		l.footers = append(l.footers, rect{c.x0, c.bottom + 0.05, c.x1 - c.x0, h - c.bottom - 0.12})
	}
	return l
}

func layouts() []*layoutSpec {
	s4 := stripSlots(4, 2, 0.15, 0.1, 1.2)
	two := func(h float64) []Slot {
		return []Slot{{X: 0.15, Y: 0.15, W: 3.7, H: h, Photo: 0}, {X: 0.15, Y: 0.25 + h, W: 3.7, H: h, Photo: 1}}
	}
	return []*layoutSpec{
		newLayout("strip4", "2×6 strip", "strip", 2, 6, s4),
		newLayout("strip3", "2×6 strip", "strip", 2, 6, stripSlots(3, 2, 0.15, 0.1, 1.55)),
		newLayout("strip2", "2×6 strip", "strip", 2, 6, stripSlots(2, 2, 0.15, 0.12, 2.4)),
		newLayout("twoup4", "4×6 two-up", "sheet", 4, 6, twoUp(s4, 2)),
		newLayout("sheet6", "4×6 sheet", "sheet", 4, 6, grid(2, 3, 4, 6, 0.15, 0.1, 0.3)),
		newLayout("sheet4", "4×6 sheet", "sheet", 4, 6, grid(2, 2, 4, 6, 0.15, 0.1, 0.6)),
		newLayout("sheet2", "4×6 sheet", "sheet", 4, 6, two(2.55)),
		newLayout("p57", "5×7 portrait", "sheet", 5, 7, grid(2, 2, 5, 7, 0.2, 0.12, 0.7)),
		newLayout("single", "4×6 single", "single", 4, 6, []Slot{{X: 0.15, Y: 0.15, W: 3.7, H: 5.2}}),
	}
}

type design struct {
	ID, Name, Category, Desc string
	Style                    Style
	Variants                 []Variant
	Deco                     func(l *layoutSpec) []Layer
	// footer text drawn in every footer area that is tall enough
	FooterText, FooterFont, FooterColor string
	FooterSize                          float64
	// FooterIcon reserves this much height above the footer text for an icon
	// (drawn by Deco) in footers tall enough to hold both.
	FooterIcon float64
}

const iconMinFooter = 0.55

func (d design) footerLayers(l *layoutSpec) []Layer {
	if d.FooterText == "" {
		return nil
	}
	var out []Layer
	for i, f := range l.footers {
		if !f.ok(0.2) {
			continue
		}
		if d.FooterIcon > 0 && f.h >= iconMinFooter {
			f.y += d.FooterIcon
			f.h -= d.FooterIcon
		}
		size := d.FooterSize
		if f.h < 0.45 {
			size = min(size, f.h*0.42)
		}
		out = append(out, Layer{ID: fmt.Sprintf("footer-%d", i+1), Name: "Footer text", Kind: LayerText, X: f.x, Y: f.y, W: f.w, H: f.h,
			Text: d.FooterText, Font: d.FooterFont, SizeIn: size, Align: "center", Color: d.FooterColor})
	}
	return out
}

func pal(bg, accent, text, border string) Style {
	return Style{Background: bg, Accent: accent, TextColor: text, BorderCol: border}
}

func v(id, name, bg, accent, text string) Variant {
	return Variant{ID: id, Name: name, Background: bg, Accent: accent, TextColor: text}
}

func across(l *layoutSpec, id string, under bool, shape string, size, elemH, gap float64, colors []string, op float64) Layer {
	return Layer{ID: id, Name: "Pattern", Kind: LayerPattern, X: 0.04, Y: 0.04, W: l.W - 0.08, H: l.H - 0.08, Under: under,
		Shape: shape, Size: size, ElemH: elemH, Gap: gap, Spread: true, Colors: colors, Opacity: op}
}

func designs() []design {
	st := func(s Style, border float64, corner float64) Style { s.BorderIn, s.CornerIn = border, corner; return s }
	return []design{
		{ID: "white-minimal", Name: "White Minimal", Category: CatClassic, Desc: "Clean white frame, thin line of text underneath.",
			Style: st(pal("#ffffff", "#9a9a9a", "#555555", "#ffffff"), 0, 0), FooterText: "{caption|date}", FooterFont: "sans", FooterColor: "text", FooterSize: 0.13,
			Variants: []Variant{v("white", "White", "#ffffff", "#9a9a9a", "#555555"), v("ivory", "Ivory", "#fbf7ee", "#a39a85", "#5c5646"), v("mist", "Mist", "#eef1f5", "#8a96a6", "#4d5866")}},
		{ID: "black-minimal", Name: "Black Minimal", Category: CatClassic, Desc: "Black frame, white lettering.",
			Style: st(pal("#101010", "#8c8c8c", "#e8e8e8", "#101010"), 0, 0), FooterText: "{caption|date}", FooterFont: "sans", FooterColor: "text", FooterSize: 0.13,
			Variants: []Variant{v("black", "Black", "#101010", "#8c8c8c", "#e8e8e8"), v("charcoal", "Charcoal", "#2b2b30", "#9a9aa4", "#ececf0"), v("navy", "Navy", "#101a33", "#7f90bd", "#e6ebf7")}},
		{ID: "clean-border", Name: "Clean Border", Category: CatClassic, Desc: "White frame with a fine outline around the strip.",
			Style: st(pal("#ffffff", "#222222", "#333333", "#222222"), 0.015, 0), FooterText: "{caption|date}", FooterFont: "sans", FooterColor: "text", FooterSize: 0.13,
			Deco: func(l *layoutSpec) []Layer {
				return []Layer{{ID: "outline", Name: "Outline", Kind: LayerRect, X: 0.05, Y: 0.05, W: l.W - 0.1, H: l.H - 0.1, StrokeIn: 0.014, Color: "accent"}}
			},
			Variants: []Variant{v("ink", "Ink", "#ffffff", "#222222", "#333333"), v("rose", "Rose", "#fffafa", "#d96b8a", "#8a3a52"), v("blue", "Blue", "#fbfdff", "#3b6fb6", "#27476f")}},
		{ID: "soft-gray", Name: "Soft Gray", Category: CatClassic, Desc: "Soft gray with rounded white photo frames.",
			Style: st(pal("#e9eaec", "#9ea1a8", "#5d6068", "#ffffff"), 0.05, 0.06), FooterText: "{caption|date}", FooterFont: "sans", FooterColor: "text", FooterSize: 0.13,
			Variants: []Variant{v("gray", "Soft Gray", "#e9eaec", "#9ea1a8", "#5d6068"), v("warm", "Warm Gray", "#ebe7e2", "#a89f94", "#65604f"), v("cool", "Cool Gray", "#e3e9ee", "#93a2b1", "#4c5966")}},

		{ID: "rose-pink", Name: "Rose Pink", Category: CatPastel, Desc: "Soft pink with scattered hearts and rounded photos.",
			Style: st(pal("#f9d5dc", "#e8869f", "#b04a68", "#ffffff"), 0.045, 0.08), FooterText: "{caption|date}", FooterFont: "bold", FooterColor: "text", FooterSize: 0.14,
			Deco: func(l *layoutSpec) []Layer {
				return []Layer{across(l, "hearts", true, "heart", 0.21, 0.2, 0.26, []string{"accent", "#ffffff"}, 0.6)}
			},
			Variants: []Variant{v("rose", "Rose", "#f9d5dc", "#e8869f", "#b04a68"), v("blush", "Blush", "#fde8ec", "#f0a6b8", "#b8566f"), v("coral", "Coral", "#f9c4b4", "#ee7f66", "#a4402c")}},
		{ID: "lavender", Name: "Lavender", Category: CatPastel, Desc: "Lavender with twinkling sparkles.",
			Style: st(pal("#e3d9f5", "#9b82d4", "#6a4fa8", "#ffffff"), 0.04, 0.06), FooterText: "{caption|date}", FooterFont: "bold", FooterColor: "text", FooterSize: 0.14,
			Deco: func(l *layoutSpec) []Layer {
				return []Layer{across(l, "sparkles", true, "sparkle", 0.22, 0.22, 0.24, []string{"accent", "#ffffff"}, 0.7)}
			},
			Variants: []Variant{v("lavender", "Lavender", "#e3d9f5", "#9b82d4", "#6a4fa8"), v("lilac", "Lilac", "#f0e6fa", "#b99be0", "#7a54b0"), v("violet", "Violet", "#cdbbf0", "#7b5fc4", "#4e3490")}},
		{ID: "cream-beige", Name: "Cream Beige", Category: CatPastel, Desc: "Warm cream with a fine polka dot.",
			Style: st(pal("#f6ecdc", "#c9a77c", "#8a6a43", "#ffffff"), 0.05, 0.03), FooterText: "{caption|date}", FooterFont: "italic", FooterColor: "text", FooterSize: 0.15,
			Deco: func(l *layoutSpec) []Layer {
				return []Layer{{ID: "dots", Name: "Polka dots", Kind: LayerPattern, X: 0.04, Y: 0.04, W: l.W - 0.08, H: l.H - 0.08, Under: true, Shape: "dot", Size: 0.05, Gap: 0.17, Color: "accent", Opacity: 0.45}}
			},
			Variants: []Variant{v("cream", "Cream", "#f6ecdc", "#c9a77c", "#8a6a43"), v("latte", "Latte", "#e9d8bf", "#b08d62", "#6f512c"), v("oat", "Oat", "#f2eadf", "#bba98f", "#7a6a52")}},
		{ID: "sky-blue", Name: "Sky Blue", Category: CatPastel, Desc: "Sky blue with soft clouds.",
			Style: st(pal("#d6ecfb", "#6fb1e0", "#3d7fb0", "#ffffff"), 0.04, 0.08), FooterText: "{caption|date}", FooterFont: "bold", FooterColor: "text", FooterSize: 0.14,
			Deco: func(l *layoutSpec) []Layer {
				return []Layer{across(l, "clouds", true, "cloud", 0.34, 0.2, 0.35, []string{"#ffffff"}, 0.85)}
			},
			Variants: []Variant{v("sky", "Sky", "#d6ecfb", "#6fb1e0", "#3d7fb0"), v("baby", "Baby Blue", "#e6f4fd", "#8cc4ea", "#4b88b5"), v("ocean", "Ocean", "#b9dcf5", "#4a98d0", "#24628f")}},

		{ID: "film-strip", Name: "Film Strip", Category: CatRetro, Desc: "Dark film stock with sprocket holes.",
			Style: st(pal("#161616", "#f2a33a", "#f2a33a", "#2e2e2e"), 0.012, 0), FooterText: "{caption|date}", FooterFont: "mono", FooterColor: "text", FooterSize: 0.12,
			Deco: func(l *layoutSpec) []Layer {
				var out []Layer
				for i, c := range l.cols {
					for j, x := range []float64{c[0] - 0.115, c[1] + 0.04} {
						out = append(out, Layer{ID: fmt.Sprintf("sprockets-%d-%d", i+1, j+1), Name: "Sprocket holes", Kind: LayerPattern, X: x, Y: 0.06, W: 0.075, H: l.H - 0.12,
							Shape: "sprocket", Size: 0.075, ElemH: 0.11, Gap: 0.09, Color: "#f3efe6", Opacity: 0.9, Under: true})
					}
				}
				return out
			},
			Variants: []Variant{v("classic", "Classic", "#161616", "#f2a33a", "#f2a33a"), v("amber", "Amber", "#2b1d0e", "#f0c27a", "#f0c27a"), v("blue", "Blue Night", "#10202e", "#8fc7ee", "#8fc7ee")}},
		{ID: "vintage-camera", Name: "Vintage Camera", Category: CatRetro, Desc: "Kraft paper, thick white prints and a little camera.",
			Style: st(pal("#e8dcc3", "#8a5a3c", "#6e4a31", "#ffffff"), 0.07, 0.01), FooterText: "{caption|date}", FooterFont: "italic", FooterColor: "text", FooterSize: 0.13, FooterIcon: 0.3,
			Deco: func(l *layoutSpec) []Layer {
				var out []Layer
				for i, f := range l.footers {
					if f.h < iconMinFooter {
						continue
					}
					cx, y := f.x+f.w/2, f.y+0.02
					out = append(out,
						Layer{ID: fmt.Sprintf("cam-body-%d", i), Name: "Camera", Kind: LayerRect, X: cx - 0.2, Y: y + 0.05, W: 0.4, H: 0.24, RadiusIn: 0.04, Color: "accent"},
						Layer{ID: fmt.Sprintf("cam-top-%d", i), Name: "Camera top", Kind: LayerRect, X: cx - 0.09, Y: y, W: 0.18, H: 0.08, RadiusIn: 0.02, Color: "accent"},
						Layer{ID: fmt.Sprintf("cam-lens-%d", i), Name: "Camera lens", Kind: LayerEllipse, X: cx - 0.085, Y: y + 0.095, W: 0.17, H: 0.17, Color: "bg"},
						Layer{ID: fmt.Sprintf("cam-glass-%d", i), Name: "Lens glass", Kind: LayerEllipse, X: cx - 0.045, Y: y + 0.135, W: 0.09, H: 0.09, Color: "accent"})
				}
				return out
			},
			Variants: []Variant{v("kraft", "Kraft", "#e8dcc3", "#8a5a3c", "#6e4a31"), v("sepia", "Sepia", "#d9c3a0", "#6e4a2a", "#573a21"), v("olive", "Olive", "#d8d9bd", "#6a6f3a", "#4d5128")}},
		{ID: "polaroid", Name: "Polaroid Inspired", Category: CatRetro, Desc: "Instant-print look: white borders with a soft shadow.",
			Style: st(pal("#d9d5cc", "#8c867a", "#5d584d", "#ffffff"), 0.09, 0.015), FooterText: "{caption|date}", FooterFont: "italic", FooterColor: "text", FooterSize: 0.14,
			Deco: func(l *layoutSpec) []Layer {
				var out []Layer
				seen := map[[2]float64]bool{}
				for i, s := range l.Slots {
					k := [2]float64{s.X, s.Y}
					if seen[k] {
						continue
					}
					seen[k] = true
					out = append(out, Layer{ID: fmt.Sprintf("shadow-%d", i), Name: "Shadow", Kind: LayerRect, X: s.X + 0.03, Y: s.Y + 0.045, W: s.W, H: s.H, RadiusIn: 0.02, Color: "#000000", Opacity: 0.16, Under: true})
				}
				return out
			},
			Variants: []Variant{v("stone", "Stone", "#d9d5cc", "#8c867a", "#5d584d"), v("peach", "Peach", "#f5d9c8", "#c98a68", "#8c5238"), v("mint", "Mint", "#cfe8dc", "#6fa88c", "#3f6e58")}},
		{ID: "monochrome-film", Name: "Monochrome Film", Category: CatRetro, Desc: "Black-and-white contact-sheet style with frame numbers.",
			Style: st(pal("#0d0d0d", "#bdbdbd", "#e2e2e2", "#8a8a8a"), 0.01, 0), FooterText: "{caption|date}", FooterFont: "mono", FooterColor: "text", FooterSize: 0.12,
			Deco: func(l *layoutSpec) []Layer {
				var out []Layer
				seen := map[[2]float64]bool{}
				n := 0
				for _, s := range l.Slots {
					k := [2]float64{s.X, s.Y}
					if seen[k] {
						continue
					}
					seen[k] = true
					n++
					out = append(out, Layer{ID: fmt.Sprintf("frameno-%d", n), Name: fmt.Sprintf("Frame number %d", n), Kind: LayerText, X: s.X + 0.06, Y: s.Y + 0.04, W: 0.5, H: 0.14,
						Text: fmt.Sprintf("%dA", n), Font: "mono", SizeIn: 0.09, Align: "left", Color: "#ffffff", Opacity: 0.8})
				}
				return out
			},
			Variants: []Variant{v("black", "Black", "#0d0d0d", "#bdbdbd", "#e2e2e2"), v("graphite", "Graphite", "#2c2c2c", "#c9c9c9", "#ededed"), v("paper", "Paper", "#efefef", "#777777", "#333333")}},

		{ID: "floral", Name: "Floral", Category: CatSpecial, Desc: "Spring blossoms scattered around the photos.",
			Style: st(pal("#fdf1ee", "#e58f9a", "#a64d5c", "#ffffff"), 0.05, 0.05), FooterText: "{caption|date}", FooterFont: "italic", FooterColor: "text", FooterSize: 0.15,
			Deco: func(l *layoutSpec) []Layer {
				return []Layer{across(l, "flowers", true, "flower", 0.26, 0.26, 0.2, []string{"accent", "#f7c6cf", "#ffffff"}, 0.85)}
			},
			Variants: []Variant{v("blossom", "Blossom", "#fdf1ee", "#e58f9a", "#a64d5c"), v("lilac", "Lilac Bloom", "#f3ecfa", "#a98bd6", "#6a4a9c"), v("sun", "Sunflower", "#fff6dc", "#f0b429", "#946200")}},
		{ID: "birthday", Name: "Birthday", Category: CatSpecial, Desc: "Confetti and a party greeting.",
			Style: st(pal("#fff4d6", "#e0457b", "#d63a6e", "#ffffff"), 0.05, 0.04), FooterText: "{caption|HAPPY BIRTHDAY}", FooterFont: "bold", FooterColor: "text", FooterSize: 0.14,
			Deco: func(l *layoutSpec) []Layer {
				cols := []string{"#ff6b8b", "#4cc9f0", "#ffd166", "#6bd490"}
				return []Layer{
					across(l, "confetti-sq", true, "square", 0.07, 0.07, 0.3, cols, 0.9),
					across(l, "confetti-tri", true, "triangle", 0.1, 0.1, 0.42, cols, 0.9),
					across(l, "confetti-dot", true, "dot", 0.06, 0.06, 0.34, cols, 0.9),
				}
			},
			Variants: []Variant{v("party", "Party", "#fff4d6", "#e0457b", "#d63a6e"), v("blue", "Blue Party", "#e2f3ff", "#2f80c9", "#1f5c94"), v("mint", "Mint Party", "#e1f7ec", "#2fa36b", "#1f7a50")}},
		{ID: "couple", Name: "Couple", Category: CatSpecial, Desc: "Hearts for two.",
			Style: st(pal("#fbe4e8", "#e0607e", "#b8325a", "#ffffff"), 0.05, 0.07), FooterText: "{caption|Us}", FooterFont: "italic", FooterColor: "text", FooterSize: 0.17, FooterIcon: 0.27,
			Deco: func(l *layoutSpec) []Layer {
				out := []Layer{across(l, "hearts", true, "heart", 0.2, 0.19, 0.34, []string{"accent", "#ffffff", "#f5b3c2"}, 0.55)}
				for i, f := range l.footers {
					if f.h < iconMinFooter {
						continue
					}
					cx, y := f.x+f.w/2, f.y+0.0
					out = append(out,
						Layer{ID: fmt.Sprintf("heart-a-%d", i), Name: "Heart", Kind: LayerShape, Shape: "heart", X: cx - 0.2, Y: y, W: 0.24, H: 0.22, Color: "accent"},
						Layer{ID: fmt.Sprintf("heart-b-%d", i), Name: "Heart", Kind: LayerShape, Shape: "heart", X: cx - 0.04, Y: y + 0.05, W: 0.24, H: 0.22, Color: "#ffffff", Opacity: 0.95})
				}
				return out
			},
			Variants: []Variant{v("rose", "Rose", "#fbe4e8", "#e0607e", "#b8325a"), v("wine", "Wine", "#f3c9d2", "#b8325a", "#7c1d3c"), v("night", "Night", "#2a1530", "#ff8fb1", "#ffd6e3")}},
		{ID: "seasonal", Name: "Seasonal", Category: CatSpecial, Desc: "Four seasonal colour schemes with stars and sparkles.",
			Style: st(pal("#fde7ee", "#f08fb0", "#b34a6e", "#ffffff"), 0.045, 0.05), FooterText: "{caption|date}", FooterFont: "bold", FooterColor: "text", FooterSize: 0.14,
			Deco: func(l *layoutSpec) []Layer {
				return []Layer{
					across(l, "stars", true, "star", 0.14, 0.14, 0.38, []string{"accent", "#ffffff"}, 0.65),
					across(l, "sparkles", true, "sparkle", 0.11, 0.11, 0.5, []string{"#ffffff", "accent"}, 0.8),
				}
			},
			Variants: []Variant{v("spring", "Spring", "#fde7ee", "#f08fb0", "#b34a6e"), v("summer", "Summer", "#fff3c4", "#f5a623", "#a86a00"), v("autumn", "Autumn", "#f6e0c8", "#c4622d", "#8a3d14"), v("winter", "Winter", "#e3eefa", "#5b8fc7", "#2f5d94")}},

		{ID: "plain", Name: "Plain", Category: CatPlain, Desc: "No decoration: just the photos.",
			Style:    st(pal("#ffffff", "#999999", "#444444", "#ffffff"), 0, 0),
			Variants: []Variant{v("white", "White", "#ffffff", "#999999", "#444444"), v("black", "Black", "#000000", "#999999", "#dddddd"), v("cream", "Cream", "#f7f1e3", "#a99f88", "#5c5440")}},
	}
}

// defaultOffered is the minimal set a new installation shows to guests: the undecorated
// Plain frame for each photo count (4, 3, 2 and 1 photos, and the 6-photo sheet). Every
// other design stays in Studio → Frames, switched off until the operator enables it.
func defaultOffered(design, layout string) bool {
	if design != "plain" {
		return false
	}
	switch layout {
	case "strip4", "strip3", "strip2", "single", "sheet6":
		return true
	}
	return false
}

// DefaultFrameID is the frame preselected for a new installation.
const DefaultFrameID = "plain.strip4"

// Designs returns the catalogue: every design on every layout.
func designCatalogue() []Template {
	var out []Template
	ls := layouts()
	for di, d := range designs() {
		for li, l := range ls {
			style := d.Style
			style.Variants = append([]Variant(nil), d.Variants...)
			if d.Deco != nil {
				style.Layers = append(style.Layers, d.Deco(l)...)
			}
			style.Layers = append(style.Layers, d.footerLayers(l)...)
			if len(style.Layers) == 0 {
				style.Layers = nil
			}
			t := Template{
				ID: d.ID + "." + l.ID, Name: d.Name, Kind: l.Kind, Category: d.Category, Description: l.Label + ": " + d.Desc,
				Order: di*100 + li, Revision: 1, Builtin: true, Caption: d.FooterText != "", Design: d.ID, LayoutID: l.ID,
				Enabled:  defaultOffered(d.ID, l.ID),
				Geometry: Geometry{WidthIn: l.W, HeightIn: l.H, DPI: 300, Slots: append([]Slot(nil), l.Slots...)},
				Style:    style,
			}
			out = append(out, t)
		}
	}
	return out
}

// Builtin returns every shipped frame: the generated catalogue plus the
// legacy layouts of earlier releases (hidden from the Booth).
func Builtin() []Template {
	return append(designCatalogue(), legacyBuiltin()...)
}
