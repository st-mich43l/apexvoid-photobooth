package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
)

func sessionsFrameRef(id string) sessions.FrameRef { return sessions.FrameRef{ID: id, Name: id} }

func shootWith(t *testing.T, s *Service, frame, variant, caption string) capture.Snapshot {
	t.Helper()
	if err := s.StartPreview(); err != nil {
		t.Fatal(err)
	}
	if err := s.StartShooting(frame, variant, caption); err != nil {
		t.Fatal(err)
	}
	return waitShoot(t, s, capture.Review)
}

func finish(t *testing.T, s *Service) capture.Snapshot {
	t.Helper()
	if err := s.AcceptPhotos(); err != nil {
		t.Fatal(err)
	}
	return waitShoot(t, s, capture.Completed)
}

func TestListFramesOffersOnlyEnabledNonLegacyFrames(t *testing.T) {
	s, _, _ := newTestApp(t)
	r := s.ListFrames(false)
	if len(r.Frames) < 5 || len(r.Categories) < 1 || r.Default == "" {
		t.Fatalf("%d frames, categories %v, default %q", len(r.Frames), r.Categories, r.Default)
	}
	for _, f := range r.Frames {
		if f.Legacy || !f.Enabled || f.Photos < 1 || f.Format == "" || f.PixelW == 0 {
			t.Fatalf("%+v", f.ID)
		}
	}
	if all := s.ListFrames(true); len(all.Frames) <= len(r.Frames) {
		t.Fatal("the operator view must also list disabled frames")
	}
	if err := s.SetFrameEnabled("lavender.sheet6", true); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range s.ListFrames(false).Frames {
		found = found || f.ID == "lavender.sheet6"
	}
	if !found {
		t.Fatal("an enabled frame must be offered")
	}
}

func TestChosenFrameDecidesTheShotCountAndIsKeptWithTheSession(t *testing.T) {
	s, root, _ := newTestApp(t)
	connectMock(t, s)
	if err := s.SetFrameEnabled("rose-pink.strip3", true); err != nil {
		t.Fatal(err)
	}
	sn := shootWith(t, s, "rose-pink.strip3", "coral", "")
	if sn.Shots != 3 || len(sn.Taken) != 3 {
		t.Fatalf("a three-photo frame must run a three-shot session: %+v", sn)
	}
	done := finish(t, s)
	sess, err := s.store.Session(done.SessionID)
	if err != nil || sess.FrameID != "rose-pink.strip3" || sess.VariantID != "coral" || sess.FrameName != "Rose Pink" || sess.FrameDir == "" {
		t.Fatalf("%v %+v", err, sess)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(sess.FrameDir), "frame.json")); err != nil {
		t.Fatalf("the frame snapshot is not stored with the session: %v", err)
	}
	m, _ := s.store.Media(done.ResultID)
	if m.FrameID != "rose-pink.strip3" || m.VariantID != "coral" || m.FrameDir != sess.FrameDir || m.Width != 600 || m.Height != 1800 {
		t.Fatalf("the finished image must say which frame made it: %+v", m)
	}
}

func TestMissingDisabledOrBadFramesAreRefusedWithoutStartingASession(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	if err := s.StartPreview(); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{{"nope", ""}, {"white-minimal.strip3", ""}, {"strip-2x6-4", ""}, {"rose-pink.strip4", "no-such-colour"}, {"", ""}} {
		err := s.StartShooting(c[0], c[1], "")
		ue, ok := err.(*UserError)
		if !ok || ue.Action == "" {
			t.Fatalf("%v: want an actionable error, got %v", c, err)
		}
	}
	if list, _ := s.store.List(10, 0); len(list) != 0 {
		t.Fatalf("a refused frame must not create a session: %+v", list)
	}
	if st := s.ShootState().State; st != capture.Preview {
		t.Fatalf("the engine must stay in preview, was %s", st)
	}
}

func TestSessionKeepsItsFrameWhenTheFrameIsLaterChangedOrDeleted(t *testing.T) {
	s, root, _ := newTestApp(t)
	connectMock(t, s)
	tdir := filepath.Join(root, "templates")
	custom := `{"id":"mine","name":"Mine","category":"Custom","widthIn":2,"heightIn":6,"dpi":300,"background":"#102030","captionEnabled":true,
	 "slots":[{"x":0.15,"y":0.2,"w":1.7,"h":2.6,"photo":0},{"x":0.15,"y":3,"w":1.7,"h":2.6,"photo":1}],
	 "layers":[{"id":"t","kind":"text","x":0,"y":5.7,"w":2,"h":0.3,"text":"{caption}","sizeIn":0.15}]}`
	if err := os.WriteFile(filepath.Join(tdir, "mine.json"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	s.tpl.Reload()
	sn := shootWith(t, s, "mine", "", "Hi!")
	// while the guests review, the operator deletes the frame
	_ = os.Remove(filepath.Join(tdir, "mine.json"))
	s.tpl.Reload()
	if s.tpl.Get("mine") != nil {
		t.Fatal("frame should be gone")
	}
	done := finish(t, s)
	if done.ResultID == "" {
		t.Fatalf("the session must still render with its snapshot: %+v", done)
	}
	sess, _ := s.store.Session(sn.SessionID)
	if sess.FrameID != "mine" || sess.Caption != "Hi!" {
		t.Fatalf("%+v", sess)
	}
	// and it can be rendered again later (gallery, reprint) from the same snapshot
	id, err := s.finalizeStrip(sn.SessionID)
	if err != nil || id == done.ResultID {
		t.Fatalf("re-render from snapshot: %v", err)
	}
	// a legacy session (no snapshot, only a layout id) still resolves
	if err := s.store.SetFrame(sn.SessionID, sessionsFrameRef("strip-2x6-2")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.finalizeStrip(sn.SessionID); err != nil {
		t.Fatalf("legacy layout id must keep working: %v", err)
	}
}

func TestSecondSessionUsesItsOwnFrameNotTheFirstOnes(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	shootWith(t, s, "plain.strip2", "", "")
	a := finish(t, s)
	_ = s.ResetShooting()
	shootWith(t, s, "plain.strip3", "black", "")
	b := finish(t, s)
	ma, _ := s.store.Media(a.ResultID)
	mb, _ := s.store.Media(b.ResultID)
	if ma.FrameID != "plain.strip2" || mb.FrameID != "plain.strip3" || mb.VariantID != "black" {
		t.Fatalf("%+v %+v", ma, mb)
	}
}

func TestChangingTheDesignAfterCaptureNeedsNoRetake(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	if err := s.SetFrameEnabled("film-strip.strip4", true); err != nil {
		t.Fatal(err)
	}
	sn := shootWith(t, s, "rose-pink.strip4", "", "")
	before := append([]capture.Shot(nil), sn.Taken...)
	// during review: another four-photo design, no retake
	if err := s.SetSessionFrame(sn.SessionID, "film-strip.strip4", "amber", "", false); err != nil {
		t.Fatal(err)
	}
	done := finish(t, s)
	m, _ := s.store.Media(done.ResultID)
	if m.FrameID != "film-strip.strip4" || m.VariantID != "amber" {
		t.Fatalf("the accepted result must use the changed design: %+v", m)
	}
	for i, sh := range done.Taken {
		if sh.MediaID != before[i].MediaID {
			t.Fatal("changing the design must not replace any photograph")
		}
	}
	// after completion: yet another design makes a second image from the same originals
	m2, err := s.ComposeSession(sn.SessionID, "plain.twoup4", "cream", "", false)
	if err != nil || m2.FrameID != "plain.twoup4" || m2.Width != 1200 {
		t.Fatalf("%v %+v", err, m2)
	}
	if _, err := s.store.Media(done.ResultID); err != nil {
		t.Fatal("the earlier composition must be kept")
	}
}

func TestFrameWithADifferentPhotoCountIsExplainedNeverSubstituted(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	sn := shootWith(t, s, "plain.strip4", "", "")
	finish(t, s)
	cases := []struct {
		frame    string
		useFirst bool
		wantErr  string
	}{
		{"plain.sheet6", false, "needs 6 photos but only 4"},
		{"plain.sheet6", true, "needs 6 photos but only 4"},
		{"plain.strip3", false, "uses 3 photos but 4 were taken"},
		{"plain.strip3", true, ""},
	}
	for _, c := range cases {
		_, err := s.ComposeSession(sn.SessionID, c.frame, "", "", c.useFirst)
		if c.wantErr == "" {
			if err != nil {
				t.Fatalf("%+v: %v", c, err)
			}
			continue
		}
		ue, ok := err.(*UserError)
		if !ok || !strings.Contains(ue.Msg, c.wantErr) || ue.Action == "" {
			t.Fatalf("%+v: %v", c, err)
		}
	}
	// the refusals did not change the session's frame
	sess, _ := s.store.Session(sn.SessionID)
	if sess.FrameID != "plain.strip3" {
		t.Fatalf("only the accepted choice may change the frame: %+v", sess)
	}
}

func TestNoFrameChangeWhilePhotosAreBeingTaken(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	cfg := s.settings.Get()
	cfg.CountdownSeconds = 10
	if err := s.settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	_ = s.StartPreview()
	if err := s.StartShooting("plain.strip4", "", ""); err != nil {
		t.Fatal(err)
	}
	id := s.ShootState().SessionID
	if err := s.SetSessionFrame(id, "plain.strip3", "", "", true); err == nil {
		t.Fatal("frame change during the countdown must be refused")
	}
	_ = s.CancelShooting()
}

func TestInterruptedCompositionKeepsPhotosAndCanBeRecomposed(t *testing.T) {
	s, root, _ := newTestApp(t)
	connectMock(t, s)
	sn := shootWith(t, s, "plain.strip4", "", "")
	sess, _ := s.store.Session(sn.SessionID)
	// the snapshot is damaged (power cut, disk error) before the strip is made
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(sess.FrameDir), "frame.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := finish(t, s)
	if done.ResultID != "" || done.Warning == "" || done.WarningCode != "composition_failed" {
		t.Fatalf("a failed composition must warn and leave no half-made image: %+v", done)
	}
	if n, _ := s.countPhotos(sn.SessionID); n != 4 {
		t.Fatalf("photos lost: %d", n)
	}
	m, err := s.ComposeSession(sn.SessionID, "plain.strip4", "", "", false)
	if err != nil || m.ID == "" {
		t.Fatalf("recovery: %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "sessions", sn.SessionID, "frames", "*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temporary snapshot folders left behind: %v", leftovers)
	}
}

func TestFramePreviewIsServedWithTokenAndRendersTheRealFrame(t *testing.T) {
	s, _, _ := newTestApp(t)
	base, tok := s.httpd.base, s.httpd.token
	if r, _ := http.Get(base + "/frame/rose-pink.strip4/-?w=200"); r == nil || r.StatusCode != 403 {
		t.Fatal("frame previews need the token")
	}
	r, err := http.Get(base + "/frame/rose-pink.strip4/coral?w=240&t=" + tok)
	if err != nil || r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("%v %v", err, r)
	}
	if r, _ := http.Get(base + "/frame/nope/-?t=" + tok); r.StatusCode != 404 {
		t.Fatal("unknown frame must 404")
	}
	if r, _ := http.Get(base + "/frame/rose-pink.strip4/..%2F..?t=" + tok); r.StatusCode == 200 {
		t.Fatal("path tricks must not work")
	}
}

func TestGuestFrameOrderingUsesCatalogueMetadata(t *testing.T) {
	s, _, _ := newTestApp(t)
	offered := s.ListFrames(false).Frames
	if len(offered) < 2 {
		t.Fatal("expected offered frames")
	}
	a, b := offered[0].ID, offered[1].ID
	if err := s.SetFrameCatalogue([]string{b, a}, map[string]bool{b: true}); err != nil {
		t.Fatal(err)
	}
	after := s.ListFrames(false).Frames
	if after[0].ID != b || after[1].ID != a || !after[0].Favorite {
		t.Fatalf("guest ordering %+v", after[:2])
	}
}
