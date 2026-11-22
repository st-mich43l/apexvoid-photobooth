package app

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

// FrameView is a frame as the UI sees it: the full definition plus derived facts.
type FrameView struct {
	templates.Template
	Photos int    `json:"photos"` // photos the frame needs → the shot count of a session using it
	Format string `json:"format"`
	PixelW int    `json:"pixelW"`
	PixelH int    `json:"pixelH"`
}

// FramesResult lists frames and the problems found in user frame files.
type FramesResult struct {
	Frames     []FrameView `json:"frames"`
	Categories []string    `json:"categories"`
	Errors     []string    `json:"errors"`
	Dir        string      `json:"dir"`
	Default    string      `json:"default"` // preselected frame id
}

// viewOf describes a frame for the UI. dpi (0 = none) is the output resolution that applies to built-in frames.
func viewOf(t templates.Template, dpi int) FrameView {
	if t.Builtin && dpi > 0 {
		t.DPI = dpi
	}
	w, h := t.PixelSize()
	return FrameView{Template: t, Photos: t.Photos(), Format: t.Format(), PixelW: w, PixelH: h}
}

// ListFrames returns the frames guests can choose (or, for the Studio frame
// manager, every frame including disabled ones). Legacy layouts are never listed.
func (s *Service) ListFrames(includeDisabled bool) FramesResult {
	res := FramesResult{Frames: []FrameView{}, Categories: []string{}, Errors: s.tpl.Errs, Dir: s.db.Layout.TemplatesDir()}
	if res.Errors == nil {
		res.Errors = []string{}
	}
	seen := map[string]bool{}
	dpi := s.settings.Get().OutputDPI
	for _, t := range s.tpl.All() {
		if t.Legacy || (!includeDisabled && !t.Enabled) {
			continue
		}
		res.Frames = append(res.Frames, viewOf(t, dpi))
		seen[t.Category] = true
	}
	for _, c := range templates.Categories {
		if seen[c] {
			res.Categories = append(res.Categories, c)
			delete(seen, c)
		}
	}
	var rest []string
	for c := range seen {
		rest = append(rest, c)
	}
	sort.Strings(rest)
	res.Categories = append(res.Categories, rest...)
	res.Default = s.settings.Get().TemplateID
	return res
}

// SetFrameEnabled offers or withdraws a frame in Booth Mode (Studio).
func (s *Service) SetFrameEnabled(id string, enabled bool) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	if err := s.tpl.SetEnabled(id, enabled); err != nil {
		return newUserErr(err.Error(), "")
	}
	s.frames.purge()
	return nil
}

// ---- preview cache ----

type frameCache struct {
	mu    sync.Mutex
	m     map[string][]byte
	order []string
}

func (c *frameCache) get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[k]
	return b, ok
}

func (c *frameCache) put(k string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string][]byte{}
	}
	if _, ok := c.m[k]; !ok {
		c.order = append(c.order, k)
	}
	c.m[k] = b
	for len(c.order) > 400 {
		delete(c.m, c.order[0])
		c.order = c.order[1:]
	}
}

func (c *frameCache) purge() {
	c.mu.Lock()
	c.m, c.order = nil, nil
	c.mu.Unlock()
}

// frameJPEG renders a frame preview (sample photos, never camera images) with
// the same renderer that makes the final media.
func (s *Service) frameJPEG(id, variant, caption string, width int) ([]byte, error) {
	if width < 60 {
		width = 60
	}
	if width > 1600 {
		width = 1600
	}
	t := s.tpl.Get(id)
	if t == nil {
		return nil, fmt.Errorf("unknown frame %q", id)
	}
	day := time.Now().Format("2006-01-02")
	key := fmt.Sprintf("%s|%s|%d|%s|%d|%s", id, variant, t.Revision, caption, width, day)
	if b, ok := s.frames.get(key); ok {
		return b, nil
	}
	r, err := t.WithVariant(variant)
	if err != nil {
		return nil, err
	}
	if !t.Caption {
		caption = ""
	}
	img, err := templates.Preview(r, s.tpl.Assets(id), templates.RenderContext{Caption: caption, Date: time.Now()}, width)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	s.frames.put(key, buf.Bytes())
	return buf.Bytes(), nil
}

// PreviewFrame returns a preview as a data URL (width in pixels).
func (s *Service) PreviewFrame(id, variant, caption string, width int) (string, error) {
	b, err := s.frameJPEG(id, variant, caption, width)
	if err != nil {
		return "", newUserErr(err.Error(), "")
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b), nil
}

// PreviewTemplate is kept for older callers: a small preview of frame id.
func (s *Service) PreviewTemplate(id string) (string, error) { return s.PreviewFrame(id, "", "", 240) }

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// ---- per-session frame ----

func (s *Service) sessionFramesDir(sessionID string) string {
	return filepath.Join(s.db.Layout.SessionDir(sessionID), "frames")
}

// validateChoice resolves the frame a guest or operator picked and applies the
// rules that never change silently: it must exist and be offered.
func (s *Service) validateChoice(frameID, variantID string) (*templates.Template, error) {
	t := s.tpl.Get(frameID)
	if t == nil {
		return nil, newUserErr("That frame is no longer available.", "Choose another frame.")
	}
	if t.Legacy || !t.Enabled {
		return nil, newUserErr("“"+t.Name+"” is not available right now.", "Choose another frame.")
	}
	if _, err := t.WithVariant(variantID); err != nil {
		return nil, newUserErr("That colour is not available for “"+t.Name+"”.", "Choose another colour.")
	}
	if err := t.Validate(); err != nil {
		return nil, newUserErr("“"+t.Name+"” has a problem and cannot be used: "+err.Error(), "Ask the host to fix or disable this frame.")
	}
	if n := t.Photos(); n < 1 || n > 12 {
		return nil, newUserErr("“"+t.Name+"” needs an unsupported number of photos.", "Choose another frame.")
	}
	return t, nil
}

// freezeFrame takes an immutable snapshot of the frame for the session and
// makes it the session's current frame. Earlier snapshots stay on disk.
func (s *Service) freezeFrame(sessionID, frameID, variantID, caption string) (templates.Snapshot, error) {
	s.frameMu.Lock()
	defer s.frameMu.Unlock()
	sess, err := s.store.Session(sessionID)
	if err != nil {
		return templates.Snapshot{}, err
	}
	dir := s.sessionFramesDir(sessionID)
	n := 0
	if es, err := os.ReadDir(dir); err == nil {
		for _, e := range es {
			if e.IsDir() && !strings.HasSuffix(e.Name(), ".tmp") {
				n++
			}
		}
	}
	snapDir := filepath.Join(dir, fmt.Sprintf("%03d", n+1))
	date := time.Now()
	if t, err := time.Parse(time.RFC3339Nano, sess.CreatedAt); err == nil {
		date = t.Local()
	}
	snap, err := s.tpl.TakeSnapshot(snapDir, frameID, variantID, caption, date, s.settings.Get().OutputDPI)
	if err != nil {
		return templates.Snapshot{}, err
	}
	rel, err := s.db.Layout.Rel(snapDir)
	if err != nil {
		return templates.Snapshot{}, err
	}
	if err := s.store.SetFrame(sessionID, sessions.FrameRef{ID: snap.FrameID, Name: snap.Name, Variant: snap.VariantID, Caption: snap.Caption, Dir: rel, Revision: snap.Revision}); err != nil {
		return templates.Snapshot{}, err
	}
	return snap, nil
}

// beginSession is the capture engine hook: freeze the chosen frame before any shot.
func (s *Service) beginSession(sessionID string, cfg capture.Config) error {
	if _, err := s.freezeFrame(sessionID, cfg.FrameID, cfg.VariantID, cfg.Caption); err != nil {
		return newUserErr("Could not prepare the frame: "+err.Error(), "Choose the frame again.")
	}
	return nil
}

// sessionFrame loads the frame a session renders with: its snapshot, or for
// sessions made before frames existed the layout id stored with the session.
func (s *Service) sessionFrame(sessionID string) (templates.Template, fs.FS, templates.RenderContext, sessions.FrameRef, error) {
	sess, err := s.store.Session(sessionID)
	if err != nil {
		return templates.Template{}, nil, templates.RenderContext{}, sessions.FrameRef{}, err
	}
	if sess.FrameDir != "" {
		abs, err := s.db.Layout.Abs(sess.FrameDir)
		if err != nil {
			return templates.Template{}, nil, templates.RenderContext{}, sessions.FrameRef{}, err
		}
		snap, assets, err := templates.ReadSnapshot(abs)
		if err != nil {
			return templates.Template{}, nil, templates.RenderContext{}, sessions.FrameRef{}, fmt.Errorf("the frame of this session cannot be read: %w", err)
		}
		return snap.Frame, assets, snap.Context(), sessions.FrameRef{ID: snap.FrameID, Name: snap.Name, Variant: snap.VariantID, Caption: snap.Caption, Dir: sess.FrameDir, Revision: snap.Revision}, nil
	}
	t := s.tpl.Get(sess.TemplateID)
	if t == nil {
		return templates.Template{}, nil, templates.RenderContext{}, sessions.FrameRef{}, fmt.Errorf("layout %q is no longer available", sess.TemplateID)
	}
	d := time.Now()
	if c, err := time.Parse(time.RFC3339Nano, sess.CreatedAt); err == nil {
		d = c.Local()
	}
	return *t, s.tpl.Assets(t.ID), templates.RenderContext{Date: d}, sessions.FrameRef{ID: t.ID, Name: t.Name, Revision: t.Revision}, nil
}

// photoImages loads the session's active photos in slot order (edited version when present).
func (s *Service) photoImages(sessionID string) ([]image.Image, error) {
	ms, err := s.store.ListMedia(sessionID, 100, 0)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		img, err := media.Decode(data)
		if err != nil {
			return nil, err
		}
		imgs = append(imgs, img)
	}
	return imgs, nil
}

type sessionsPhoto struct {
	slot int
	id   string
}

// finalizeStrip composes the session's photos into its own frame snapshot and
// stores the result as a separate "strip" media item. Originals are only read.
func (s *Service) finalizeStrip(sessionID string) (string, error) {
	t, assets, rc, ref, err := s.sessionFrame(sessionID)
	if err != nil {
		return "", err
	}
	imgs, err := s.photoImages(sessionID)
	if err != nil {
		return "", err
	}
	data, w, h, err := templates.Render(t, imgs, assets, rc)
	if err != nil {
		return "", err
	}
	m, err := s.store.AddFrameExport(sessionID, "strip", t.ID, data, w, h, ref)
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

// SetSessionFrame changes the design of a session whose photos are already
// taken, without retaking anything. The new frame must use the number of photos
// that were taken; a frame that needs fewer is only used when useFirst is true
// (the extra photos stay in the library). It never substitutes another frame.
func (s *Service) SetSessionFrame(sessionID, frameID, variantID, caption string, useFirst bool) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	if snap := s.eng.Snapshot(); snap.SessionID == sessionID {
		switch snap.State {
		case capture.Countdown, capture.Capturing, capture.Saving, capture.Preview:
			return newUserErr("Photos are still being taken.", "Change the frame after the photos are shot.")
		}
	}
	t, err := s.validateChoice(frameID, variantID)
	if err != nil {
		return err
	}
	have, err := s.countPhotos(sessionID)
	if err != nil {
		return userErr(err)
	}
	need := t.Photos()
	switch {
	case have == 0:
		return newUserErr("This session has no photos.", "")
	case need > have:
		return newUserErr(fmt.Sprintf("“%s” needs %d photos but only %d were taken.", t.Name, need, have), fmt.Sprintf("Choose a frame for %d photos, or start over with this frame.", have))
	case need < have && !useFirst:
		return newUserErr(fmt.Sprintf("“%s” uses %d photos but %d were taken.", t.Name, need, have), fmt.Sprintf("Use the first %d photos, or choose a frame for %d photos.", need, have))
	}
	if _, err := s.freezeFrame(sessionID, frameID, variantID, caption); err != nil {
		return userErr(err)
	}
	return nil
}

func (s *Service) countPhotos(sessionID string) (int, error) {
	ms, err := s.store.ListMedia(sessionID, 100, 0)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range ms {
		if m.Kind == "photo" {
			n++
		}
	}
	return n, nil
}

// ComposeSession gives a finished session another design: it switches the
// session to the chosen frame and renders a new image from the original photos.
// The earlier compositions stay in the library.
func (s *Service) ComposeSession(sessionID, frameID, variantID, caption string, useFirst bool) (sessions.Media, error) {
	if err := s.SetSessionFrame(sessionID, frameID, variantID, caption, useFirst); err != nil {
		return sessions.Media{}, err
	}
	id, err := s.finalizeStrip(sessionID)
	if err != nil {
		return sessions.Media{}, newUserErr("Could not compose the photos: "+err.Error(), "Your original photos are safe. Try again.")
	}
	m, err := s.store.Media(id)
	return m, userErr(err)
}
