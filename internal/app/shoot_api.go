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

// StartShooting: PREVIEW -> COUNTDOWN with a configured pool of candidate
// photos. The chosen frame determines how many of those photos must be picked
// for its slots, and is frozen into the session before the first shot.
func (s *Service) StartShooting(frameID, variantID, caption string) error {
	return s.startShooting(frameID, variantID, caption, 0)
}

// StartShootingWithCount lets operator diagnostics request a short capture
// without changing the configured guest session count.
func (s *Service) StartShootingWithCount(frameID, variantID, caption string, count int) error {
	return s.startShooting(frameID, variantID, caption, count)
}

func (s *Service) startShooting(frameID, variantID, caption string, requestedCount int) error {
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
	shots := cfg.Shots
	if requestedCount > 0 {
		shots = requestedCount
	}
	if shots < t.Photos() {
		shots = t.Photos()
	}
	return userErr(s.eng.Start(capture.Config{Shots: shots, FramePhotos: t.Photos(), Countdown: cfg.CountdownSeconds, TemplateID: t.ID, FrameID: t.ID, VariantID: variantID, Caption: caption}))
}

// RetakePhoto re-shoots one slot (0-based) during review.
func (s *Service) RetakePhoto(slot int) error { return userErr(s.eng.Retake(slot)) }

// AcceptPhotos: REVIEW -> SAVING -> COMPLETED.
func (s *Service) AcceptPhotos() error { return userErr(s.eng.Accept()) }

// AcceptSelectedPhotos orders the selected capture slots first so the frame
// renderer places the guest's choices into its photo windows.
func (s *Service) AcceptSelectedPhotos(slots []int) error {
	return userErr(s.eng.AcceptSelected(slots))
}

// CancelShooting aborts the running session.
func (s *Service) CancelShooting() error { return userErr(s.eng.Cancel()) }

// ResetShooting returns the engine to IDLE (live view off).
func (s *Service) ResetShooting() error { return userErr(s.eng.Reset()) }
