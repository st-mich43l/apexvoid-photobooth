package templates

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

func twoUp(base []Slot, stripW float64) []Slot { // two identical strips side by side (cut apart)
	out := append([]Slot{}, base...)
	for _, s := range base {
		s.X += stripW
		out = append(out, s)
	}
	return out
}

func lt(id, name, kind string, w, h float64, bg string, slots []Slot) Template {
	return Template{ID: id, Name: name, Kind: kind, Category: CatLegacy, Legacy: true, Builtin: true, Revision: 1,
		Geometry: Geometry{WidthIn: w, HeightIn: h, DPI: 300, Slots: slots}, Style: Style{Background: bg}}
}

// legacyBuiltin returns the layouts of the earlier releases. They stay
// resolvable (old sessions and settings refer to their ids) but are not offered
// in Booth Mode.
func legacyBuiltin() []Template {
	s4 := stripSlots(4, 2, 0.15, 0.1, 1.2)
	a := lt("strip-2x6-4", "Korean strip 2×6 — 4 photos", "strip", 2, 6, "#14101f", s4)
	a.BorderIn, a.BorderCol = 0.02, "#f4a6c0"
	b := lt("strip-4x6-2up", "4×6 sheet — two 2×6 strips (cut apart)", "sheet", 4, 6, "#14101f", twoUp(s4, 2))
	b.BorderIn, b.BorderCol = 0.02, "#f4a6c0"
	return []Template{
		a,
		lt("strip-2x6-3", "Strip 2×6 — 3 photos", "strip", 2, 6, "#ffffff", stripSlots(3, 2, 0.15, 0.1, 1.6)),
		lt("strip-2x6-2", "Strip 2×6 — 2 photos", "strip", 2, 6, "#fdf2f6", stripSlots(2, 2, 0.15, 0.12, 2.4)),
		b,
		lt("sheet-4x6-6", "4×6 sheet — 6 photos", "sheet", 4, 6, "#ffffff", grid(2, 3, 4, 6, 0.15, 0.1, 0.3)),
		lt("single-5x7", "Single photo 7×5 landscape", "single", 7, 5, "#ffffff", []Slot{{X: 0.2, Y: 0.2, W: 6.6, H: 4.6}}),
		lt("single-5x7-portrait", "Single photo 5×7 portrait", "single", 5, 7, "#ffffff", []Slot{{X: 0.2, Y: 0.2, W: 4.6, H: 6.6}}),
		lt("single-4x6", "Single photo 6×4 landscape", "single", 6, 4, "#ffffff", []Slot{{X: 0.15, Y: 0.15, W: 5.7, H: 3.7}}),
		lt("single-4x6-portrait", "Single photo 4×6 portrait", "single", 4, 6, "#ffffff", []Slot{{X: 0.15, Y: 0.15, W: 3.7, H: 5.7}}),
	}
}

// ForShots picks the legacy default layout for a shot count.
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
