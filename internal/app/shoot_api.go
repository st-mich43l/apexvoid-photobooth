package app

import (
	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
)

// ShootState returns the shooting engine state.
func (s *Service) ShootState() capture.Snapshot { return s.eng.Snapshot() }

// StartPreview: IDLE -> PREVIEW (turns live view on).
func (s *Service) StartPreview() error {
	ctx, cancel := s.ctx()
	defer cancel()
	return userErr(s.eng.StartPreview(ctx))
}

// StartShooting: PREVIEW -> COUNTDOWN with the frame the guest chose. The
// frame decides how many photos are taken; it is frozen into the session
// before the first shot and is never replaced by another one.
func (s *Service) StartShooting(frameID, variantID, caption string) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	if err := s.startCheck(); err != nil {
		return err
	}
	t, err := s.validateChoice(frameID, variantID)
	if err != nil {
		return err
	}
	cfg := s.settings.Get()
	return userErr(s.eng.Start(capture.Config{Shots: t.Photos(), Countdown: cfg.CountdownSeconds, TemplateID: t.ID, FrameID: t.ID, VariantID: variantID, Caption: caption}))
}

// RetakePhoto re-shoots one slot (0-based) during review.
func (s *Service) RetakePhoto(slot int) error { return userErr(s.eng.Retake(slot)) }

// AcceptPhotos: REVIEW -> SAVING -> COMPLETED.
func (s *Service) AcceptPhotos() error { return userErr(s.eng.Accept()) }

// CancelShooting aborts the running session.
func (s *Service) CancelShooting() error { return userErr(s.eng.Cancel()) }

// ResetShooting returns the engine to IDLE (live view off).
func (s *Service) ResetShooting() error { return userErr(s.eng.Reset()) }
