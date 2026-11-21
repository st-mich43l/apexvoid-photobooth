// Package printing is the printer layer: discovery, capabilities, page
// placement, preview, submission through the OS print system, spooler job
// tracking, persistence and duplicate protection. Windows printing goes
// through the Windows spooler and GDI (winspool_windows.go) using the
// installed printer driver; CUPS is used elsewhere.
//
// Nothing here can prove that paper came out of a printer: the spooler only
// reports that a job left the queue. The UI says so.
package printing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// ErrNotImplemented is returned by drivers that cannot do something on this platform.
var ErrNotImplemented = errors.New("not implemented for this platform")

// Printer is an installed printer.
type Printer struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
	Status  string `json:"status"` // ready | offline | error | unknown
	Detail  string `json:"detail,omitempty"`
	Port    string `json:"port,omitempty"`
	Driver  string `json:"driver,omitempty"`
}

// Available reports whether jobs can be sent now.
func (p Printer) Available() bool { return p.Status == "ready" }

// Paper is a physical paper size.
type Paper struct {
	ID       int     `json:"id"` // driver paper id (DMPAPER_*), 0 = by name / custom
	Name     string  `json:"name"`
	WidthIn  float64 `json:"widthIn"`
	HeightIn float64 `json:"heightIn"`
}

// Orientation of the page.
type Orientation string

const (
	Auto      Orientation = "auto" // follow the image
	Portrait  Orientation = "portrait"
	Landscape Orientation = "landscape"
)

// Fit decides how the image is placed on the page.
type Fit string

const (
	FitPrintable Fit = "fit"    // whole image inside the printable area (default; never cut)
	FitFill      Fit = "fill"   // cover the whole sheet, cropping the overflow (borderless-style)
	FitActual    Fit = "actual" // physical size from the image DPI, centred
)

// Caps are the capabilities a printer reports.
type Caps struct {
	Papers       []Paper       `json:"papers"`
	Orientations []Orientation `json:"orientations"`
	MaxCopies    int           `json:"maxCopies"`
	Color        bool          `json:"color"`
	Resolutions  [][2]int      `json:"resolutions"`
	CustomPaper  bool          `json:"customPaper"`
	Note         string        `json:"note,omitempty"`
}

// KnownPapers are used when a printer reports none (and for CUPS / tests).
var KnownPapers = []Paper{
	{0, "4x6", 4, 6}, {0, "5x7", 5, 7}, {0, "6x8", 6, 8}, {0, "6x9", 6, 9}, {0, "8x10", 8, 10},
	{0, "A6", 4.13, 5.83}, {0, "A5", 5.83, 8.27}, {0, "A4", 8.27, 11.69}, {0, "Letter", 8.5, 11},
}

// FindPaper looks up by exact name (case-insensitive), then by size.
func FindPaper(papers []Paper, name string) (Paper, bool) {
	for _, p := range papers {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Paper{}, false
}

// FitsPaper reports whether a w×h inch image fits the paper in either
// orientation without scaling up (0.05 in tolerance).
func FitsPaper(widthIn, heightIn float64, p Paper) bool {
	fit := func(w, h float64) bool { return w <= p.WidthIn+0.05 && h <= p.HeightIn+0.05 }
	return fit(widthIn, heightIn) || fit(heightIn, widthIn)
}

// ValidatePaper checks that an image of w×h inches fits the named paper.
func ValidatePaper(widthIn, heightIn float64, paper string) error {
	return ValidateAgainst(KnownPapers, widthIn, heightIn, paper)
}

// ValidateAgainst checks against an explicit paper list (a printer's own sizes).
func ValidateAgainst(papers []Paper, widthIn, heightIn float64, paper string) error {
	p, ok := FindPaper(papers, paper)
	if !ok {
		return fmt.Errorf("unknown paper size %q", paper)
	}
	if !FitsPaper(widthIn, heightIn, p) {
		return fmt.Errorf("a %.3gx%.3g in layout does not fit %s paper (%.3gx%.3g in)", widthIn, heightIn, p.Name, p.WidthIn, p.HeightIn)
	}
	return nil
}

// SameAspect reports whether two sizes have the same shape within tol (either orientation).
func SameAspect(w1, h1, w2, h2, tol float64) bool {
	a, b := w1/h1, w2/h2
	return math.Abs(a-b)/b <= tol || math.Abs(1/a-b)/b <= tol
}

// Job statuses. "Completed" means the spooler itself reported PRINTED/COMPLETE;
// "removed" means the job left the queue (usually printed) — neither proves paper.
const (
	StatusSubmitting = "submitting"
	StatusQueued     = "queued"
	StatusPrinting   = "printing"
	StatusCompleted  = "completed"
	StatusRemoved    = "removed"
	StatusFailed     = "failed"
	StatusUnknown    = "unknown"
)

// Terminal reports whether a status can no longer change by itself.
func Terminal(s string) bool {
	switch s {
	case StatusCompleted, StatusRemoved, StatusFailed, StatusUnknown:
		return true
	}
	return false
}

// SubmitRequest is one print submission to a driver. File is the final,
// print-ready rendered image (never a raw camera original).
type SubmitRequest struct {
	Printer     string
	File        string
	Title       string
	Paper       Paper
	Orientation Orientation
	Fit         Fit
	Copies      int
	ImageDPI    int
}

// JobState is a spooler observation.
type JobState struct {
	Status string
	Detail string
	Pages  int
}

// PageGeometry is the printer's page in device pixels for a chosen paper and orientation.
type PageGeometry struct {
	PhysW, PhysH   int // whole sheet
	PrintW, PrintH int // printable area
	OffX, OffY     int // printable origin inside the sheet
	DPIX, DPIY     int
	Assumed        bool // estimated, not read from the driver
}

// Driver talks to the OS print system.
type Driver interface {
	Name() string
	List(ctx context.Context) ([]Printer, error)
	Capabilities(ctx context.Context, printer string) (Caps, error)
	Geometry(ctx context.Context, printer string, paper Paper, o Orientation) (PageGeometry, error)
	Submit(ctx context.Context, r SubmitRequest) (nativeID string, err error)
	Status(ctx context.Context, printer, nativeID string) (JobState, error)
}

// Job is a tracked, persisted print job.
type Job struct {
	ID              string    `json:"id"`
	RequestID       string    `json:"requestId"`
	ParentID        string    `json:"parentId"` // set on reprints: the job being repeated
	Kind            string    `json:"kind"`     // print | test
	MediaID         string    `json:"mediaId"`
	RenderedMediaID string    `json:"renderedMediaId"`
	SessionID       string    `json:"sessionId"`
	TemplateID      string    `json:"templateId"`
	File            string    `json:"file"`
	Printer         string    `json:"printer"`
	PaperName       string    `json:"paperName"`
	PaperID         int       `json:"paperId"`
	Orientation     string    `json:"orientation"`
	Fit             string    `json:"fit"`
	Copies          int       `json:"copies"`
	Status          string    `json:"status"`
	Detail          string    `json:"detail"`
	NativeID        string    `json:"nativeId"`
	Attempt         int       `json:"attempt"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
