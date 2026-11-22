package app

import (
	"bytes"
	"encoding/base64"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

// The Frame Builder API (Studio). Guests never reach it: Booth Mode only
// lists frames and starts sessions.

// GetFrame returns the full definition of a frame for editing.
func (s *Service) GetFrame(id string) (templates.Template, error) {
	t := s.tpl.Get(id)
	if t == nil {
		return templates.Template{}, newUserErr("That frame no longer exists.", "")
	}
	return *t, nil
}

// NewFrameDraft returns an unsaved editable copy of a frame (any built-in
// layout/design or custom frame) with a fresh id. Nothing is written yet.
func (s *Service) NewFrameDraft(sourceID, name string) (templates.Template, error) {
	d, err := s.tpl.NewDraft(sourceID, strings.TrimSpace(name))
	if err != nil {
		return templates.Template{}, newUserErr(err.Error(), "")
	}
	return d, nil
}

// ImportFrameImage validates a PNG/JPEG chosen by the operator (base64, with
// or without a data: prefix) and stores a private copy. The returned name is
// what the frame refers to; the original path is never kept.
func (s *Service) ImportFrameImage(b64 string) (templates.AssetInfo, error) {
	if err := s.guardWrite(); err != nil {
		return templates.AssetInfo{}, err
	}
	if i := strings.Index(b64, ","); i >= 0 && strings.HasPrefix(b64, "data:") {
		b64 = b64[i+1:]
	}
	if len(b64) > (templates.MaxAssetBytes*4)/3+16 {
		return templates.AssetInfo{}, newUserErr("The image is too large.", "Use a file under 15 MB.")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return templates.AssetInfo{}, newUserErr("The image could not be read.", "Choose the file again.")
	}
	info, err := s.tpl.Staging().Import(data)
	if err != nil {
		return templates.AssetInfo{}, newUserErr(err.Error(), "Use a PNG or JPEG file of at most 15 MB and 40 megapixels.")
	}
	return info, nil
}

// draftAssets resolves a draft's images: freshly imported ones first, then the
// ones the saved frame already owns.
func (s *Service) draftAssets(id string) templates.Layered {
	ls := templates.Layered{s.tpl.Staging().FS()}
	if t := s.tpl.Get(id); t != nil && t.Editable {
		ls = append(ls, os.DirFS(filepath.Join(s.db.Layout.TemplatesDir(), "custom", id, "assets")))
	}
	return ls
}

// PreviewDraft renders an unsaved frame with sample photos (same renderer as
// the final output) as a JPEG data URL.
func (s *Service) PreviewDraft(t templates.Template, variantID, caption string, width int) (string, error) {
	width = min(max(width, 80), 1400)
	r, err := t.WithVariant(variantID)
	if err != nil {
		return "", newUserErr(err.Error(), "")
	}
	if !t.Caption {
		caption = ""
	}
	img, err := templates.Preview(r, s.draftAssets(t.ID), templates.RenderContext{Caption: caption, Date: time.Now()}, width)
	if err != nil {
		return "", newUserErr(err.Error(), "")
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		return "", userErr(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// SaveFrame validates and stores a custom frame (new or edited). Sessions that
// already used an earlier version are unaffected: they hold their own snapshot.
func (s *Service) SaveFrame(t templates.Template) (FrameView, error) {
	if err := s.guardWrite(); err != nil {
		return FrameView{}, err
	}
	saved, err := s.tpl.SaveCustom(t, s.tpl.Staging().FS())
	if err != nil {
		return FrameView{}, newUserErr(err.Error(), "Fix the highlighted problem and save again.")
	}
	s.frames.purge()
	s.log.Info("frame saved", "id", saved.ID, "revision", saved.Revision)
	return viewOf(saved), nil
}

// DeleteFrame removes a custom frame. Existing sessions keep rendering, and no
// photo is touched.
func (s *Service) DeleteFrame(id string) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	if err := s.tpl.DeleteCustom(id); err != nil {
		return newUserErr(err.Error(), "")
	}
	if cfg := s.settings.Get(); cfg.TemplateID == id {
		cfg.TemplateID = templates.DefaultFrameID
		_ = s.settings.Save(cfg)
		s.emit("settings:changed", s.publicSettings())
	}
	s.frames.purge()
	s.log.Info("frame deleted", "id", id)
	return nil
}
