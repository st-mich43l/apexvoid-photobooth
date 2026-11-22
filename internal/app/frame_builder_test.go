package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

func b64PNG(w, h int, c color.NRGBA) string {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestOperatorBuildsACustomFrameWithTransparentOverlayAndGuestsCanUseIt(t *testing.T) {
	s, root, _ := newTestApp(t)
	connectMock(t, s)
	info, err := s.ImportFrameImage("data:image/png;base64," + b64PNG(600, 1800, color.NRGBA{255, 0, 255, 60}))
	if err != nil || !info.HasAlpha {
		t.Fatalf("%v %+v", err, info)
	}
	d, err := s.NewFrameDraft("plain.strip4", "Gold Night")
	if err != nil {
		t.Fatal(err)
	}
	// four photo slots of the built-in layout are kept; the operator moves one and adds the overlay
	d.Slots[0].X, d.Slots[0].Y = 0.2, 0.2
	d.Overlay, d.Background, d.Enabled, d.Caption = info.Name, "#101010", true, true
	if _, err := s.PreviewDraft(d, "", "", 300); err != nil {
		t.Fatalf("the draft must preview before it is saved: %v", err)
	}
	if s.tpl.Get(d.ID) != nil {
		t.Fatal("a draft must not exist in the library until it is saved")
	}
	v, err := s.SaveFrame(d)
	if err != nil || v.Photos != 4 || v.Revision != 1 || !v.Editable {
		t.Fatalf("%v %+v", err, v)
	}
	// it is offered in Booth Mode and a session can use it
	offered := false
	for _, f := range s.ListFrames(false).Frames {
		offered = offered || f.ID == v.ID
	}
	if !offered {
		t.Fatal("an enabled custom frame must appear in Booth Mode")
	}
	shootWith(t, s, v.ID, "", "Mai & Nam")
	done := finish(t, s)
	m, _ := s.store.Media(done.ResultID)
	if m.FrameID != v.ID || m.Width != 600 || m.Height != 1800 {
		t.Fatalf("%+v", m)
	}
	if _, err := os.Stat(filepath.Join(root, "templates", "custom", v.ID, "assets", info.Name)); err != nil {
		t.Fatal("the overlay must be stored inside the frame:", err)
	}
	// edit the frame afterwards: the finished session keeps its snapshot (revision 1)
	d2, _ := s.GetFrame(v.ID)
	d2.Background = "#ffffff"
	v2, err := s.SaveFrame(d2)
	if err != nil || v2.Revision != 2 {
		t.Fatalf("%v %+v", err, v2)
	}
	sess, _ := s.store.Session(done.SessionID)
	if sess.FrameRev != 1 {
		t.Fatalf("session must keep revision 1, has %d", sess.FrameRev)
	}
	if _, err := s.finalizeStrip(done.SessionID); err != nil {
		t.Fatal(err)
	}
	// and deleting it later still leaves the session printable/renderable
	if err := s.DeleteFrame(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.finalizeStrip(done.SessionID); err != nil {
		t.Fatalf("session must survive deletion of its frame: %v", err)
	}
	if err := s.StartShooting(v.ID, "", ""); err == nil {
		t.Fatal("a deleted frame cannot start a new session")
	}
}

func TestImportRejectsBadFilesWithAnActionableMessage(t *testing.T) {
	s, _, _ := newTestApp(t)
	for name, in := range map[string]string{
		"svg":     base64.StdEncoding.EncodeToString([]byte(`<svg onload="alert(1)"/>`)),
		"garbage": "!!!not base64!!!",
		"empty":   "",
		"huge":    strings.Repeat("A", 25<<20),
	} {
		_, err := s.ImportFrameImage(in)
		ue, ok := err.(*UserError)
		if !ok || ue.Msg == "" || ue.Action == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCustomFrameIsNotUsableUntilEnabledAndDeletionResetsTheDefault(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	_ = s.StartPreview()
	d, _ := s.NewFrameDraft("plain.strip2", "Hidden")
	v, err := s.SaveFrame(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StartShooting(v.ID, "", ""); err == nil {
		t.Fatal("a custom frame the operator has not enabled must be refused")
	}
	if err := s.SetFrameEnabled(v.ID, true); err != nil {
		t.Fatal(err)
	}
	cfg := s.settings.Get()
	cfg.TemplateID = v.ID
	_ = s.settings.Save(cfg)
	if err := s.DeleteFrame(v.ID); err != nil {
		t.Fatal(err)
	}
	if s.settings.Get().TemplateID != templates.DefaultFrameID {
		t.Fatal("the preselected frame must fall back when it is deleted")
	}
	if err := s.DeleteFrame("rose-pink.strip4"); err == nil {
		t.Fatal("built-in frames cannot be deleted")
	}
	_ = capture.Preview
}

func TestBuilderWritesAreBlockedAfterLibraryMigration(t *testing.T) {
	s, _, _ := newTestApp(t)
	s.restartRequired.Store(true)
	d, _ := s.NewFrameDraft("plain.strip4", "X")
	if _, err := s.SaveFrame(d); err == nil {
		t.Fatal("saving must be blocked while a restart is required")
	}
	if _, err := s.ImportFrameImage(b64PNG(10, 10, color.NRGBA{})); err == nil {
		t.Fatal("importing must be blocked")
	}
}
