package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

// PrintersResult is the Printer page's list.
type PrintersResult struct {
	Printers []printing.Printer `json:"printers"`
	Driver   string             `json:"driver"`
	Default  string             `json:"default"`
	Note     string             `json:"note"`
}

const printNote = "Printing goes through the Windows spooler and the installed printer driver. The spooler can only tell that a job left the queue — it cannot prove that paper came out. Physical output has been validated only where the hardware test checklist says so."

func (s *Service) printerDriverName() string { return s.prn.Driver().Name() }

// ListPrinters returns the installed printers with availability.
func (s *Service) ListPrinters() (PrintersResult, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	ps, err := s.prn.Driver().List(ctx)
	if ps == nil {
		ps = []printing.Printer{}
	}
	def := ""
	for _, p := range ps {
		if p.Default {
			def = p.Name
		}
	}
	return PrintersResult{Printers: ps, Driver: s.printerDriverName(), Default: def, Note: printNote}, userErr(err)
}

// PrinterCapabilities reports the paper sizes, orientations and copies a printer supports.
func (s *Service) PrinterCapabilities(printer string) (printing.Caps, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	c, err := s.prn.Driver().Capabilities(ctx, printer)
	if err != nil {
		return printing.Caps{}, userErr(err)
	}
	sort.SliceStable(c.Papers, func(i, j int) bool { return c.Papers[i].Name < c.Papers[j].Name })
	return c, nil
}

// PrintOptions is one print/preview request from the UI.
type PrintOptions struct {
	RequestID   string `json:"requestId"` // new value per user action: a repeated id never prints twice
	MediaID     string `json:"mediaId"`
	TemplateID  string `json:"templateId"` // layout for single photos (and to verify a strip)
	Printer     string `json:"printer"`
	Paper       string `json:"paper"`
	Orientation string `json:"orientation"` // auto | portrait | landscape
	Fit         string `json:"fit"`         // fit | fill | actual
	Copies      int    `json:"copies"`
}

// printable is the final, print-ready image chosen for a media item.
type printable struct {
	img        image.Image
	path       string // "" until rendered to disk
	data       []byte // JPEG when rendered in memory
	widthIn    float64
	heightIn   float64
	dpi        int
	templateID string
	sessionID  string
	rendered   *sessions.Media // set when a photo was rendered through a layout and saved
}

// printSource implements the pipeline
//
//	captured images → selected layout → rendered print-ready image → driver → spooler
//
// A composed strip/print is used as it is (after checking it matches the layout
// size). A single photo is NEVER printed as the raw original: it is rendered
// through a single-photo layout (using its edited version when it has one).
// save=false renders in memory (preview) without touching the library.
func (s *Service) printSource(mediaID, templateID, paper string, save bool) (*printable, error) {
	m, err := s.store.Media(mediaID)
	if err != nil {
		return nil, newUserErr("That image could not be found.", "")
	}
	switch m.Kind {
	case "video":
		return nil, newUserErr("Videos cannot be printed.", "")
	case "strip", "print":
		p, err := s.store.FilePath(m.ID, "original")
		if err != nil {
			return nil, userErr(err)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, userErr(err)
		}
		img, err := media.Decode(data)
		if err != nil {
			return nil, userErr(err)
		}
		dpi := 300
		if u, x, _, ok := media.JPEGDensity(data); ok && u == 1 && x >= 72 {
			dpi = x
		}
		b := img.Bounds()
		pr := &printable{img: img, path: p, widthIn: float64(b.Dx()) / float64(dpi), heightIn: float64(b.Dy()) / float64(dpi), dpi: dpi, sessionID: m.SessionID, templateID: templateID}
		if t := s.tpl.Get(templateID); t != nil {
			w, h := t.PixelSize()
			if abs(w-b.Dx()) > 1 || abs(h-b.Dy()) > 1 {
				return nil, newUserErr("This image was not made with the layout “"+t.Name+"”.", "Choose the layout it was created with, or compose it again from the session in the Gallery.")
			}
		}
		return pr, nil
	}
	// a single photograph: render it through a one-photo layout
	t := s.tpl.Get(templateID)
	if t == nil {
		t = s.singlePhotoLayout(paper)
	}
	if t == nil {
		return nil, newUserErr("Choose a single-photo layout for this photo.", "Layouts: 4×6 or 5×7, portrait or landscape.")
	}
	if t.Photos() != 1 {
		return nil, newUserErr("“"+t.Name+"” needs "+fmt.Sprint(t.Photos())+" photos.", "Choose a single-photo layout, or print the finished strip from the gallery.")
	}
	path, err := s.store.FilePath(m.ID, "edited") // the edited version when there is one, else the original
	if err != nil {
		return nil, userErr(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, userErr(err)
	}
	src, err := media.Decode(data)
	if err != nil {
		return nil, userErr(err)
	}
	// choose the layout orientation to match the photo
	if sb := src.Bounds(); (sb.Dx() > sb.Dy()) != (t.WidthIn > t.HeightIn) {
		if alt := s.rotatedTwin(t); alt != nil {
			t = alt
		}
	}
	out, w, h, err := templates.Render(*t, []image.Image{src}, s.db.Layout.TemplatesDir())
	if err != nil {
		return nil, userErr(err)
	}
	img, err := media.Decode(out)
	if err != nil {
		return nil, userErr(err)
	}
	pr := &printable{img: img, data: out, widthIn: t.WidthIn, heightIn: t.HeightIn, dpi: t.DPI, templateID: t.ID, sessionID: m.SessionID}
	if save {
		rm, err := s.store.AddExport(m.SessionID, "print", "print-"+t.ID, out, w, h)
		if err != nil {
			return nil, userErr(err)
		}
		pr.rendered = &rm
		pr.path, _ = s.store.FilePath(rm.ID, "original")
	}
	return pr, nil
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// singlePhotoLayout picks a builtin one-photo layout from the paper name.
func (s *Service) singlePhotoLayout(paper string) *templates.Template {
	id := "single-4x6-portrait"
	if strings.Contains(paper, "5") && strings.Contains(paper, "7") {
		id = "single-5x7-portrait"
	}
	return s.tpl.Get(id)
}

// rotatedTwin returns the portrait/landscape counterpart of a builtin single layout.
func (s *Service) rotatedTwin(t *templates.Template) *templates.Template {
	pairs := map[string]string{"single-4x6": "single-4x6-portrait", "single-4x6-portrait": "single-4x6", "single-5x7": "single-5x7-portrait", "single-5x7-portrait": "single-5x7"}
	if id, ok := pairs[t.ID]; ok {
		return s.tpl.Get(id)
	}
	return nil
}

func (s *Service) resolvePaper(ctx context.Context, printer, name string) (printing.Paper, printing.Caps, error) {
	c, err := s.prn.Driver().Capabilities(ctx, printer)
	if err != nil {
		return printing.Paper{}, c, userErr(err)
	}
	if name == "" {
		name = s.settings.Get().PrintPaper
	}
	if p, ok := printing.FindPaper(c.Papers, name); ok {
		return p, c, nil
	}
	return printing.Paper{}, c, newUserErr(fmt.Sprintf("The printer does not offer the paper size “%s”.", name), "Choose one of the sizes listed for this printer.")
}

func orient(o string) printing.Orientation {
	switch printing.Orientation(o) {
	case printing.Portrait, printing.Landscape:
		return printing.Orientation(o)
	}
	return printing.Auto
}

func fitOf(f string) printing.Fit {
	switch printing.Fit(f) {
	case printing.FitFill, printing.FitActual:
		return printing.Fit(f)
	}
	return printing.FitPrintable
}

// PreviewResult is an accurate preview: the sheet, the printer's unprintable
// border and the image exactly where it will be placed.
type PreviewResult struct {
	Image       string     `json:"image"` // data URL
	PaperMm     [2]float64 `json:"paperMm"`
	PrintableMm [2]float64 `json:"printableMm"`
	MarginMm    [4]float64 `json:"marginMm"`
	ScalePct    float64    `json:"scalePct"`
	CroppedPct  float64    `json:"croppedPct"`
	Rotated     bool       `json:"rotated"`
	Orientation string     `json:"orientation"`
	Warning     string     `json:"warning"`
	AssumedGeom bool       `json:"assumedGeometry"`
	ImageInches [2]float64 `json:"imageInches"`
	Template    string     `json:"templateId"`
}

func (s *Service) plan(ctx context.Context, o PrintOptions, pr *printable) (printing.PageGeometry, printing.PagePlan, printing.Paper, printing.Orientation, error) {
	paper, _, err := s.resolvePaper(ctx, o.Printer, o.Paper)
	if err != nil {
		return printing.PageGeometry{}, printing.PagePlan{}, paper, "", err
	}
	b := pr.img.Bounds()
	or := orient(o.Orientation)
	if or == printing.Auto {
		or = printing.Portrait
		if b.Dx() > b.Dy() {
			or = printing.Landscape
		}
	}
	g, err := s.prn.Driver().Geometry(ctx, o.Printer, paper, or)
	if err != nil {
		return g, printing.PagePlan{}, paper, or, userErr(err)
	}
	return g, printing.PlanPage(b.Dx(), b.Dy(), pr.dpi, g, fitOf(o.Fit)), paper, or, nil
}

// PrintPreview renders the real placement for the chosen printer/paper/fit.
func (s *Service) PrintPreview(o PrintOptions) (PreviewResult, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	pr, err := s.printSource(o.MediaID, o.TemplateID, o.Paper, false)
	if err != nil {
		return PreviewResult{}, err
	}
	g, pl, _, or, err := s.plan(ctx, o, pr)
	if err != nil {
		return PreviewResult{}, err
	}
	pv := printing.Preview(pr.img, g, pl, 420)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, pv, &jpeg.Options{Quality: 85}); err != nil {
		return PreviewResult{}, userErr(err)
	}
	mm := func(px, dpi int) float64 { return float64(px) / float64(dpi) * 25.4 }
	return PreviewResult{
		Image: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), Orientation: string(or), Template: pr.templateID,
		PaperMm: [2]float64{mm(g.PhysW, g.DPIX), mm(g.PhysH, g.DPIY)}, PrintableMm: [2]float64{mm(g.PrintW, g.DPIX), mm(g.PrintH, g.DPIY)},
		MarginMm: pl.MarginMM, ScalePct: pl.ScalePct, CroppedPct: pl.CroppedPct, Rotated: pl.Rotated, Warning: pl.Warning, AssumedGeom: g.Assumed,
		ImageInches: [2]float64{pr.widthIn, pr.heightIn},
	}, nil
}

// PrintMedia renders the final layout and submits one print job.
func (s *Service) PrintMedia(o PrintOptions) (printing.Job, error) {
	if err := s.guardWrite(); err != nil { // a print may render and store a new image
		return printing.Job{}, err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	if o.RequestID == "" {
		return printing.Job{}, newUserErr("The print request has no id.", "Reload the page and try again.")
	}
	// idempotent: the same click never renders or prints twice
	if j, ok, err := s.prnStore.ByRequest(o.RequestID); err == nil && ok {
		return j, nil
	}
	pr, err := s.printSource(o.MediaID, o.TemplateID, o.Paper, true)
	if err != nil {
		return printing.Job{}, err
	}
	paper, _, err := s.resolvePaper(ctx, o.Printer, o.Paper)
	if err != nil {
		return printing.Job{}, err
	}
	req := printing.PrintRequest{RequestID: o.RequestID, Kind: "print", MediaID: o.MediaID, SessionID: pr.sessionID, TemplateID: pr.templateID, File: pr.path,
		Title: "ApexVoid Photobooth " + o.MediaID, WidthIn: pr.widthIn, HeightIn: pr.heightIn, ImageDPI: pr.dpi, Printer: o.Printer, Paper: paper,
		Orientation: orient(o.Orientation), Fit: fitOf(o.Fit), Copies: o.Copies}
	if pr.rendered != nil {
		req.RenderedMediaID = pr.rendered.ID
	}
	j, err := s.prn.Print(ctx, req)
	if err != nil {
		s.log.Warn("print failed", "printer", o.Printer, "media", o.MediaID, "err", err)
		if errors.Is(err, printing.ErrDuplicate) {
			return j, newUserErr(err.Error(), "")
		}
		return j, newUserErr(err.Error(), "Check the printer, then try again.")
	}
	s.log.Info("print submitted", "job", j.ID, "printer", j.Printer, "paper", j.PaperName, "file", j.File, "native", j.NativeID)
	s.emit("print:job", j)
	return j, nil
}

// TestPrint prints a labelled calibration page (grid, edge marks, paper size)
// so margins and scale can be checked on the real printer without using a photo.
func (s *Service) TestPrint(printer, paperName, orientation string) (printing.Job, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	paper, _, err := s.resolvePaper(ctx, printer, paperName)
	if err != nil {
		return printing.Job{}, err
	}
	or := orient(orientation)
	if or == printing.Auto {
		or = printing.Portrait
	}
	img := testPage(paper, or)
	dir := filepath.Join(s.db.Layout.Root, "exports", "test-prints")
	_ = os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "test-"+time.Now().Format("20060102-150405")+".png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return printing.Job{}, userErr(err)
	}
	if err := storage.WriteNew(file, buf.Bytes()); err != nil {
		return printing.Job{}, userErr(err)
	}
	b := img.Bounds()
	j, err := s.prn.Print(ctx, printing.PrintRequest{RequestID: "test-" + file, Kind: "test", MediaID: "test:" + paper.Name, File: file, Title: "ApexVoid test print",
		WidthIn: float64(b.Dx()) / 300, HeightIn: float64(b.Dy()) / 300, ImageDPI: 300, Printer: printer, Paper: paper, Orientation: or, Fit: printing.FitActual, Copies: 1})
	if err != nil {
		return j, newUserErr(err.Error(), "Check the printer, then try again.")
	}
	s.emit("print:job", j)
	return j, nil
}

// testPage draws a calibration page at the paper's size, 300 dpi.
func testPage(p printing.Paper, o printing.Orientation) image.Image {
	g := printing.GeometryFor(p, o, 300, 0)
	w, h := g.PhysW, g.PhysH
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	grid, dark := color.NRGBA{200, 200, 215, 255}, color.NRGBA{20, 16, 36, 255}
	step := 300 * 10 / 25 // ~10 mm
	for x := 0; x < w; x += step {
		draw.Draw(img, image.Rect(x, 0, x+1, h), image.NewUniform(grid), image.Point{}, draw.Src)
	}
	for y := 0; y < h; y += step {
		draw.Draw(img, image.Rect(0, y, w, y+1), image.NewUniform(grid), image.Point{}, draw.Src)
	}
	rect := func(r image.Rectangle, c color.Color) {
		draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
	}
	// sheet-edge frame (3 px) and centre cross
	rect(image.Rect(0, 0, w, 3), dark)
	rect(image.Rect(0, h-3, w, h), dark)
	rect(image.Rect(0, 0, 3, h), dark)
	rect(image.Rect(w-3, 0, w, h), dark)
	rect(image.Rect(w/2-1, 0, w/2+2, h), dark)
	rect(image.Rect(0, h/2-1, w, h/2+2), dark)
	// 25 mm squares in the corners: if one is cut off, that margin is unprintable
	sq := mmToPx(25)
	for _, c := range []image.Point{{0, 0}, {w - sq, 0}, {0, h - sq}, {w - sq, h - sq}} {
		rect(image.Rect(c.X, c.Y, c.X+sq, c.Y+sq), color.NRGBA{244, 166, 192, 255})
	}
	text := func(s string, x, y, scale int) {
		small := image.NewNRGBA(image.Rect(0, 0, 8*len(s)+2, 14))
		draw.Draw(small, small.Bounds(), image.NewUniform(color.NRGBA{255, 255, 255, 0}), image.Point{}, draw.Src)
		d := &font.Drawer{Dst: small, Src: image.NewUniform(dark), Face: basicfont.Face7x13, Dot: fixed.P(1, 11)}
		d.DrawString(s)
		for yy := 0; yy < small.Bounds().Dy(); yy++ {
			for xx := 0; xx < small.Bounds().Dx(); xx++ {
				if _, _, _, a := small.At(xx, yy).RGBA(); a > 0 {
					rect(image.Rect(x+xx*scale, y+yy*scale, x+(xx+1)*scale, y+(yy+1)*scale), dark)
				}
			}
		}
	}
	text("TEST PRINT - NOT A PHOTO", sq+30, h/2-140, 4)
	text(fmt.Sprintf("%s  %.2f x %.2f in  (%.0f x %.0f mm)", p.Name, float64(w)/300, float64(h)/300, float64(w)/300*25.4, float64(h)/300*25.4), sq+30, h/2-70, 3)
	text("Grid = 10 mm. Pink corner squares = 25 mm.", sq+30, h/2+40, 3)
	text("Edges cut off = unprintable margin of this printer.", sq+30, h/2+100, 3)
	text(time.Now().Format("2006-01-02 15:04"), sq+30, h/2+160, 3)
	return img
}

// PrintJobs lists the persisted job history (newest first).
func (s *Service) PrintJobs() []printing.Job {
	js, _ := s.prn.Jobs(100)
	if js == nil {
		js = []printing.Job{}
	}
	return js
}

// Reprint repeats a finished job. confirmed must be true (the UI asks first).
func (s *Service) Reprint(id string, confirmed bool) (printing.Job, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	j, err := s.prn.Reprint(ctx, id, confirmed)
	if err != nil {
		return j, newUserErr(err.Error(), "")
	}
	if _, serr := os.Stat(j.File); serr != nil {
		return j, newUserErr("The rendered print file is missing: "+j.File, "Print the image again from the gallery.")
	}
	s.emit("print:job", j)
	return j, nil
}

// pollPrints keeps job states fresh while any job is active.
func (s *Service) pollPrints(stop <-chan struct{}) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			for _, j := range s.prn.Poll(ctx) {
				s.emit("print:job", j)
			}
			cancel()
		}
	}
}

func mmToPx(mm float64) int { return int(mm / 25.4 * 300) }
