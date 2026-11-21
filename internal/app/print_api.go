package app

import (
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
)

// PrintersResult wraps discovery so the UI can show why none were found.
type PrintersResult struct {
	Printers []printing.Printer `json:"printers"`
	Driver   string             `json:"driver"`
	Papers   []printing.Paper   `json:"papers"`
	Note     string             `json:"note"`
}

// ListPrinters discovers printers. Printing is architecture-only in Phase 1.
func (s *Service) ListPrinters() (PrintersResult, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	ps, err := s.spool.Printers(ctx)
	if ps == nil {
		ps = []printing.Printer{}
	}
	return PrintersResult{Printers: ps, Papers: printing.KnownPapers, Driver: s.spoolDriverName(),
		Note: "Printer discovery is implemented. Submitting print jobs is NOT validated on any physical printer in this release (Windows submission is not implemented yet)."}, userErr(err)
}

func (s *Service) spoolDriverName() string {
	if s.opts.PrintDriver != nil {
		return s.opts.PrintDriver.Name()
	}
	return printing.NewOSDriver().Name()
}

// PrintMedia validates the layout against the paper and submits a job.
func (s *Service) PrintMedia(mediaID, printer, paper string, copies int, templateID string) (printing.Job, error) {
	t := s.tpl.Get(templateID)
	if t == nil {
		return printing.Job{}, newUserErr("Unknown layout.", "Choose the layout the image was made with.")
	}
	path, err := s.store.FilePath(mediaID, "original")
	if err != nil {
		return printing.Job{}, userErr(err)
	}
	ctx, cancel := s.ctx()
	defer cancel()
	j, err := s.spool.Print(ctx, printer, path, mediaID, t.WidthIn, t.HeightIn, printing.Options{Paper: paper, Copies: copies})
	if err != nil {
		return printing.Job{}, newUserErr(err.Error(), "")
	}
	s.emit("print:job", j)
	return j, nil
}

// PrintJobs lists tracked jobs.
func (s *Service) PrintJobs() []printing.Job { return s.spool.Jobs() }

// Reprint resubmits a job (bounded).
func (s *Service) Reprint(id string) (printing.Job, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	j, err := s.spool.Reprint(ctx, id)
	if err != nil {
		return j, newUserErr(err.Error(), "")
	}
	return j, nil
}
