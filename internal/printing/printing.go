// Package printing is the printer architecture: discovery, paper validation,
// job submission, status tracking and controlled reprinting behind a Driver
// interface. Phase 1 ships discovery (Windows PowerShell / CUPS) and CUPS
// submission; Windows submission is NOT implemented and nothing here has been
// validated against a physical printer.
package printing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ErrNotImplemented is returned by drivers that cannot submit jobs yet.
var ErrNotImplemented = errors.New("printing is not implemented for this platform yet")

// Printer is a discovered printer.
type Printer struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
	Status  string `json:"status"` // ready | offline | error | unknown
}

// Paper is a physical paper size in inches.
type Paper struct {
	Name     string  `json:"name"`
	WidthIn  float64 `json:"widthIn"`
	HeightIn float64 `json:"heightIn"`
}

// KnownPapers are the sizes a photobooth normally prints on.
var KnownPapers = []Paper{
	{"4x6", 4, 6}, {"5x7", 5, 7}, {"6x8", 6, 8}, {"6x9", 6, 9}, {"8x10", 8, 10},
	{"A6", 4.13, 5.83}, {"A5", 5.83, 8.27}, {"A4", 8.27, 11.69}, {"Letter", 8.5, 11},
}

// ValidatePaper checks that an image of w x h inches fits the named paper in
// either orientation (within 0.05 in), without scaling.
func ValidatePaper(widthIn, heightIn float64, paper string) error {
	for _, p := range KnownPapers {
		if !strings.EqualFold(p.Name, paper) {
			continue
		}
		fit := func(w, h float64) bool { return w <= p.WidthIn+0.05 && h <= p.HeightIn+0.05 }
		if fit(widthIn, heightIn) || fit(heightIn, widthIn) {
			return nil
		}
		return fmt.Errorf("a %.2gx%.2g in layout does not fit %s paper (%.2gx%.2g in)", widthIn, heightIn, p.Name, p.WidthIn, p.HeightIn)
	}
	return fmt.Errorf("unknown paper size %q", paper)
}

// Options of a print request.
type Options struct {
	Paper  string
	Copies int
}

// JobStatus values.
const (
	JobQueued    = "queued"
	JobPrinting  = "printing"
	JobCompleted = "completed"
	JobFailed    = "failed"
)

// Driver talks to the OS print system.
type Driver interface {
	Name() string
	List(ctx context.Context) ([]Printer, error)
	Submit(ctx context.Context, printer, file string, o Options) (nativeID string, err error)
	Status(ctx context.Context, nativeID string) (status string, detail string, err error)
}

// Job is a tracked print job.
type Job struct {
	ID        string    `json:"id"`
	File      string    `json:"file"`
	MediaID   string    `json:"mediaId"`
	Printer   string    `json:"printer"`
	Paper     string    `json:"paper"`
	Copies    int       `json:"copies"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	Prints    int       `json:"prints"` // submissions so far (1 + reprints)
	CreatedAt time.Time `json:"createdAt"`
	native    string
}

// Spooler validates, submits and tracks jobs.
type Spooler struct {
	drv Driver
	// MaxReprints bounds controlled reprinting per job (default 3).
	MaxReprints int

	mu   sync.Mutex
	seq  int
	jobs map[string]*Job
}

// NewSpooler wraps a driver.
func NewSpooler(d Driver) *Spooler { return &Spooler{drv: d, MaxReprints: 3, jobs: map[string]*Job{}} }

// Printers lists printers.
func (s *Spooler) Printers(ctx context.Context) ([]Printer, error) { return s.drv.List(ctx) }

// Print validates the paper against the layout size and submits the job.
func (s *Spooler) Print(ctx context.Context, printer, file, mediaID string, widthIn, heightIn float64, o Options) (Job, error) {
	if o.Copies < 1 || o.Copies > 10 {
		return Job{}, errors.New("copies must be 1..10")
	}
	if printer == "" {
		return Job{}, errors.New("no printer selected")
	}
	if err := ValidatePaper(widthIn, heightIn, o.Paper); err != nil {
		return Job{}, err
	}
	native, err := s.drv.Submit(ctx, printer, file, o)
	if err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	s.seq++
	j := &Job{ID: fmt.Sprintf("P%04d", s.seq), File: file, MediaID: mediaID, Printer: printer, Paper: o.Paper, Copies: o.Copies,
		Status: JobQueued, Prints: 1, CreatedAt: time.Now(), native: native}
	s.jobs[j.ID] = j
	cp := *j
	s.mu.Unlock()
	return cp, nil
}

// Refresh polls the driver for the job's status.
func (s *Spooler) Refresh(ctx context.Context, id string) (Job, error) {
	s.mu.Lock()
	j, ok := s.jobs[id]
	if !ok {
		s.mu.Unlock()
		return Job{}, fmt.Errorf("unknown print job %s", id)
	}
	native := j.native
	s.mu.Unlock()
	st, detail, err := s.drv.Status(ctx, native)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		j.Status, j.Detail = st, detail
	} else {
		j.Detail = err.Error()
	}
	return *j, err
}

// Reprint resubmits a finished or failed job. It is refused while the job is
// still queued/printing and after MaxReprints, so one tap cannot spool
// unlimited copies.
func (s *Spooler) Reprint(ctx context.Context, id string) (Job, error) {
	if _, err := s.Refresh(ctx, id); err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	j := s.jobs[id]
	if j.Status == JobQueued || j.Status == JobPrinting {
		s.mu.Unlock()
		return Job{}, errors.New("this job is still printing")
	}
	if j.Prints > s.MaxReprints {
		s.mu.Unlock()
		return Job{}, fmt.Errorf("reprint limit reached (%d)", s.MaxReprints)
	}
	file, printer, paper, copies := j.File, j.Printer, j.Paper, j.Copies
	s.mu.Unlock()
	native, err := s.drv.Submit(ctx, printer, file, Options{Paper: paper, Copies: copies})
	if err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j.native, j.Status, j.Detail = native, JobQueued, ""
	j.Prints++
	return *j, nil
}

// Jobs returns tracked jobs.
func (s *Spooler) Jobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.jobs))
	for i := 1; i <= s.seq; i++ {
		if j, ok := s.jobs[fmt.Sprintf("P%04d", i)]; ok {
			out = append(out, *j)
		}
	}
	return out
}
