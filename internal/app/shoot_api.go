package app

import (
	"context"
	"fmt"
	"image"
	"os"
	"sort"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

// ShootState returns the shooting engine state.
func (s *Service) ShootState() capture.Snapshot { return s.eng.Snapshot() }

// StartPreview: IDLE -> PREVIEW (turns live view on).
func (s *Service) StartPreview() error {
	ctx, cancel := s.ctx()
	defer cancel()
	return userErr(s.eng.StartPreview(ctx))
}

// StartShooting: PREVIEW -> COUNTDOWN. shots is 1, 2, 4 or 6.
func (s *Service) StartShooting(shots int) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	cfg := s.settings.Get()
	switch shots {
	case 1, 2, 4, 6:
	default:
		return newUserErr("Choose 1, 2, 4 or 6 photographs.", "")
	}
	tid := cfg.TemplateID
	if t := s.tpl.Get(tid); t == nil || t.Photos() != shots {
		tid = templates.ForShots(shots)
	}
	s.mu.Lock()
	s.tplID = tid
	s.mu.Unlock()
	return userErr(s.eng.Start(capture.Config{Shots: shots, Countdown: cfg.CountdownSeconds, TemplateID: tid}))
}

// RetakePhoto re-shoots one slot (0-based) during review.
func (s *Service) RetakePhoto(slot int) error { return userErr(s.eng.Retake(slot)) }

// AcceptPhotos: REVIEW -> SAVING -> COMPLETED.
func (s *Service) AcceptPhotos() error { return userErr(s.eng.Accept()) }

// CancelShooting aborts the running session.
func (s *Service) CancelShooting() error { return userErr(s.eng.Cancel()) }

// ResetShooting returns the engine to IDLE (live view off).
func (s *Service) ResetShooting() error { return userErr(s.eng.Reset()) }

// finalizeStrip is the engine's SAVING step: compose the layout from the
// accepted originals. A failure here never loses photos.
func (s *Service) finalizeStrip(sessionID string) (string, error) {
	sess, err := s.store.Session(sessionID)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	tid := s.tplID
	s.mu.Unlock()
	if tid == "" {
		tid = sess.TemplateID
	}
	t := s.tpl.Get(tid)
	if t == nil {
		return "", fmt.Errorf("template %q not found", tid)
	}
	ms, err := s.store.ListMedia(sessionID, 100, 0)
	if err != nil {
		return "", err
	}
	var photos []sessionsPhoto
	for _, m := range ms {
		if m.Kind == "photo" {
			photos = append(photos, sessionsPhoto{slot: m.Slot, id: m.ID})
		}
	}
	sort.Slice(photos, func(i, j int) bool { return photos[i].slot < photos[j].slot })
	imgs := make([]image.Image, 0, len(photos))
	for _, p := range photos {
		path, err := s.store.FilePath(p.id, "edited")
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		img, err := media.Decode(data)
		if err != nil {
			return "", err
		}
		imgs = append(imgs, img)
	}
	data, w, h, err := templates.Render(*t, imgs, s.db.Layout.TemplatesDir())
	if err != nil {
		return "", err
	}
	m, err := s.store.AddExport(sessionID, "strip", t.ID, data, w, h)
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

type sessionsPhoto struct {
	slot int
	id   string
}

var _ = context.Background
