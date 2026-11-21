package app

import (
	"encoding/base64"
	"image"
	"image/color"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

// TemplatesResult lists layouts plus problems found in user template files.
type TemplatesResult struct {
	Templates []templates.Template `json:"templates"`
	Errors    []string             `json:"errors"`
	Dir       string               `json:"dir"`
}

// ListTemplates returns builtin and user layouts.
func (s *Service) ListTemplates() TemplatesResult {
	errs := s.tpl.Errs
	if errs == nil {
		errs = []string{}
	}
	return TemplatesResult{Templates: s.tpl.All(), Errors: errs, Dir: s.db.Layout.TemplatesDir()}
}

// PreviewTemplate renders the layout with generated placeholder photographs
// (clearly colour blocks, not captures) at low DPI, as a data URL.
func (s *Service) PreviewTemplate(id string) (string, error) {
	t := s.tpl.Get(id)
	if t == nil {
		return "", newUserErr("Unknown layout.", "")
	}
	t.DPI = 100
	n := t.Photos()
	photos := make([]image.Image, n)
	pal := []color.NRGBA{{244, 166, 192, 255}, {150, 120, 220, 255}, {120, 190, 230, 255}, {240, 200, 120, 255}, {160, 220, 170, 255}, {230, 140, 140, 255}}
	for i := range photos {
		photos[i] = media.Solid(600, 400, pal[i%len(pal)])
	}
	data, _, _, err := templates.Render(*t, photos, s.db.Layout.TemplatesDir())
	if err != nil {
		return "", userErr(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data), nil
}

// ComposeSession renders a session's photos into a layout and stores the
// result as a "strip" media item.
func (s *Service) ComposeSession(sessionID, templateID string) (sessions.Media, error) {
	if err := s.guardWrite(); err != nil {
		return sessions.Media{}, err
	}
	s.mu.Lock()
	prev := s.tplID
	s.tplID = templateID
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.tplID = prev; s.mu.Unlock() }()
	id, err := s.finalizeStrip(sessionID)
	if err != nil {
		return sessions.Media{}, userErr(err)
	}
	m, err := s.store.Media(id)
	return m, userErr(err)
}
