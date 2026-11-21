package printing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Errors a caller can react to.
var (
	ErrDuplicate    = errors.New("an identical print was just submitted")
	ErrNeedsConfirm = errors.New("reprinting needs confirmation")
)

// Manager validates, submits and tracks print jobs on top of a Driver and a Store.
type Manager struct {
	drv   Driver
	store Store
	now   func() time.Time

	// MaxReprints bounds controlled reprinting per original job (default 3).
	MaxReprints int
	// DuplicateWindow: an identical new print inside this window is refused (default 15 s).
	DuplicateWindow time.Duration

	mu       sync.Mutex
	inflight map[string]bool
}

// NewManager creates a manager.
func NewManager(d Driver, s Store) *Manager {
	return &Manager{drv: d, store: s, now: time.Now, MaxReprints: 3, DuplicateWindow: 15 * time.Second, inflight: map[string]bool{}}
}

// SetClock overrides the clock (tests).
func (m *Manager) SetClock(f func() time.Time) { m.now = f }

// Driver returns the underlying driver.
func (m *Manager) Driver() Driver { return m.drv }

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "P" + time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// PrintRequest asks for one print of a rendered file.
type PrintRequest struct {
	RequestID       string // idempotency key generated per user action
	Kind            string // print | test
	MediaID         string
	RenderedMediaID string
	SessionID       string
	TemplateID      string
	File            string // final, print-ready image
	Title           string
	WidthIn         float64 // physical size of the image
	HeightIn        float64
	ImageDPI        int
	Printer         string
	Paper           Paper
	Orientation     Orientation
	Fit             Fit
	Copies          int
}

func dupKey(r PrintRequest) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d", r.Kind, r.MediaID, r.Printer, r.Paper.Name, r.Copies)
}

// Print submits a job. The same RequestID never prints twice; an identical
// print inside DuplicateWindow is refused (use Reprint to print again).
func (m *Manager) Print(ctx context.Context, r PrintRequest) (Job, error) {
	if r.Kind == "" {
		r.Kind = "print"
	}
	if r.Copies < 1 || r.Copies > 10 {
		return Job{}, errors.New("copies must be 1..10")
	}
	if r.Printer == "" {
		return Job{}, errors.New("no printer selected")
	}
	if r.File == "" {
		return Job{}, errors.New("there is no rendered image to print")
	}
	if r.Fit == "" {
		r.Fit = FitPrintable
	}
	if r.Orientation == "" {
		r.Orientation = Auto
	}
	// idempotency: a repeated request returns the job it already created
	if j, ok, err := m.store.ByRequest(r.RequestID); err != nil {
		return Job{}, err
	} else if ok {
		return j, nil
	}
	// physical fit (r.Paper comes from the printer's own list)
	if r.Kind != "test" && !FitsPaper(r.WidthIn, r.HeightIn, r.Paper) && r.Fit != FitFill {
		return Job{}, fmt.Errorf("a %.3gx%.3g in layout does not fit %s paper (%.3gx%.3g in)", r.WidthIn, r.HeightIn, r.Paper.Name, r.Paper.WidthIn, r.Paper.HeightIn)
	}
	// printer must be present and usable
	ps, err := m.drv.List(ctx)
	if err != nil {
		return Job{}, err
	}
	var pr *Printer
	for i := range ps {
		if ps[i].Name == r.Printer {
			pr = &ps[i]
		}
	}
	if pr == nil {
		return Job{}, fmt.Errorf("printer %q is not installed", r.Printer)
	}
	if !pr.Available() {
		msg := fmt.Sprintf("printer %q is %s", pr.Name, pr.Status)
		if pr.Detail != "" {
			msg += ": " + pr.Detail
		}
		return Job{}, errors.New(msg)
	}

	key := dupKey(r)
	m.mu.Lock()
	if m.inflight[key] {
		m.mu.Unlock()
		return Job{}, ErrDuplicate
	}
	m.inflight[key] = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(m.inflight, key); m.mu.Unlock() }()

	now := m.now()
	job := Job{ID: newID(), RequestID: r.RequestID, Kind: r.Kind, MediaID: r.MediaID, RenderedMediaID: r.RenderedMediaID, SessionID: r.SessionID,
		TemplateID: r.TemplateID, File: r.File, Printer: r.Printer, PaperName: r.Paper.Name, PaperID: r.Paper.ID, Orientation: string(r.Orientation),
		Fit: string(r.Fit), Copies: r.Copies, Status: StatusSubmitting, Detail: "Submitting to the Windows print queue…", Attempt: 1, CreatedAt: now, UpdatedAt: now}
	if prev, ok, err := m.store.Recent(job, now.Add(-m.DuplicateWindow)); err != nil {
		return Job{}, err
	} else if ok {
		return prev, fmt.Errorf("%w (%s ago); use Reprint to print again", ErrDuplicate, now.Sub(prev.CreatedAt).Round(time.Second))
	}
	return m.submit(ctx, job, r)
}

func (m *Manager) submit(ctx context.Context, job Job, r PrintRequest) (Job, error) {
	// persisted BEFORE submission: a crash in between is visible (and shown as unknown) after restart
	if err := m.store.Insert(job); err != nil {
		return Job{}, err
	}
	native, err := m.drv.Submit(ctx, SubmitRequest{Printer: r.Printer, File: r.File, Title: r.Title, Paper: r.Paper, Orientation: r.Orientation,
		Fit: r.Fit, Copies: r.Copies, ImageDPI: r.ImageDPI})
	job.UpdatedAt = m.now()
	if err != nil {
		job.Status, job.Detail = StatusFailed, "Not sent to the printer: "+err.Error()
		_ = m.store.Update(job)
		return job, err
	}
	job.NativeID, job.Status, job.Detail = native, StatusQueued, "Sent to the Windows print queue."
	if err := m.store.Update(job); err != nil {
		return job, err
	}
	return job, nil
}

// Refresh asks the driver for the job's state and stores it.
func (m *Manager) Refresh(ctx context.Context, id string) (Job, error) {
	j, err := m.store.Get(id)
	if err != nil {
		return Job{}, fmt.Errorf("unknown print job %s", id)
	}
	if Terminal(j.Status) || j.NativeID == "" {
		return j, nil
	}
	st, err := m.drv.Status(ctx, j.Printer, j.NativeID)
	if err != nil {
		return j, err
	}
	// a job that failed in the queue and was then removed stays failed
	if st.Status == StatusRemoved && j.Status == StatusFailed {
		return j, nil
	}
	if st.Status != j.Status || st.Detail != j.Detail {
		j.Status, j.Detail, j.UpdatedAt = st.Status, st.Detail, m.now()
		if err := m.store.Update(j); err != nil {
			return j, err
		}
	}
	return j, nil
}

// Poll refreshes every active job and returns those that changed.
func (m *Manager) Poll(ctx context.Context) []Job {
	active, err := m.store.Active()
	if err != nil {
		return nil
	}
	var changed []Job
	for _, j := range active {
		before := j.Status + "|" + j.Detail
		nj, err := m.Refresh(ctx, j.ID)
		if err == nil && nj.Status+"|"+nj.Detail != before {
			changed = append(changed, nj)
		}
	}
	return changed
}

// Recover runs at start-up: jobs that were active when the app stopped are
// re-checked against the spooler; one that cannot be explained becomes
// "unknown" with instructions, never silently "completed".
func (m *Manager) Recover(ctx context.Context) []Job {
	active, err := m.store.Active()
	if err != nil {
		return nil
	}
	var out []Job
	for _, j := range active {
		if j.Status == StatusSubmitting || j.NativeID == "" {
			j.Status = StatusUnknown
			j.Detail = "The app stopped while this job was being sent. Check the Windows print queue and the printer before printing again."
			j.UpdatedAt = m.now()
			_ = m.store.Update(j)
			out = append(out, j)
			continue
		}
		st, err := m.drv.Status(ctx, j.Printer, j.NativeID)
		if err != nil {
			j.Status = StatusUnknown
			j.Detail = "The app was restarted and the print queue could not be read: " + err.Error()
		} else {
			j.Status, j.Detail = st.Status, st.Detail
			if st.Status == StatusRemoved {
				j.Detail = "The job left the print queue while the app was closed. It was most likely printed; this cannot be confirmed."
			}
		}
		j.UpdatedAt = m.now()
		_ = m.store.Update(j)
		out = append(out, j)
	}
	return out
}

// Jobs lists recent jobs (newest first).
func (m *Manager) Jobs(limit int) ([]Job, error) { return m.store.List(limit) }

// Reprint repeats a finished or failed job. It needs confirmed=true, refuses
// jobs that are still in the queue, and is bounded by MaxReprints per original.
func (m *Manager) Reprint(ctx context.Context, id string, confirmed bool) (Job, error) {
	if !confirmed {
		return Job{}, ErrNeedsConfirm
	}
	j, err := m.Refresh(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if !Terminal(j.Status) {
		return Job{}, errors.New("this job is still in the print queue")
	}
	root := j.ID
	if j.ParentID != "" {
		root = j.ParentID
	}
	all, err := m.store.List(500)
	if err != nil {
		return Job{}, err
	}
	n := 0
	for _, x := range all {
		if x.ID == root || x.ParentID == root {
			n++
		}
	}
	if n > m.MaxReprints {
		return Job{}, fmt.Errorf("reprint limit reached (%d reprints)", m.MaxReprints)
	}
	ps, err := m.drv.List(ctx)
	if err != nil {
		return Job{}, err
	}
	for _, p := range ps {
		if p.Name == j.Printer && !p.Available() {
			return Job{}, fmt.Errorf("printer %q is %s", p.Name, p.Status)
		}
	}
	now := m.now()
	nj := j
	nj.ID, nj.RequestID, nj.ParentID, nj.NativeID = newID(), "", root, ""
	nj.Status, nj.Detail, nj.Attempt, nj.CreatedAt, nj.UpdatedAt = StatusSubmitting, "Submitting reprint…", n+1, now, now
	return m.submit(ctx, nj, PrintRequest{Printer: j.Printer, File: j.File, Title: "Reprint " + j.ID,
		Paper: Paper{ID: j.PaperID, Name: j.PaperName}, Orientation: Orientation(j.Orientation), Fit: Fit(j.Fit), Copies: j.Copies, ImageDPI: 300})
}
