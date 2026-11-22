package templates

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

func jpegBytes(w, h int) []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, media.Solid(w, h, color.NRGBA{200, 100, 50, 255}), nil)
	return b.Bytes()
}

func overlayPNG(w, h int) []byte { // transparent with a 50% red band and an opaque green box
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := h / 2; y < h/2+h/10; y++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 128})
		}
	}
	for x := 0; x < w/10; x++ {
		for y := 0; y < h/10; y++ {
			img.SetNRGBA(x, y, color.NRGBA{0, 255, 0, 255})
		}
	}
	return pngBytes(img)
}

func newLib(t *testing.T) *Library { return Load(t.TempDir()) }

func draftFrom(t *testing.T, l *Library, src, name string) Template {
	t.Helper()
	d, err := l.NewDraft(src, name)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCreateFromABuiltInLayoutSaveAndReload(t *testing.T) {
	l := newLib(t)
	d := draftFrom(t, l, "plain.strip4", "My Wedding")
	if d.ID != "custom-my-wedding" || d.Builtin || d.Category != CatCustom || d.Photos() != 4 || d.DPI != 300 {
		t.Fatalf("%+v", d)
	}
	d.Background = "#223344"
	d.Layers = append(d.Layers, Layer{ID: "t", Kind: LayerText, X: 0.1, Y: 5.4, W: 1.8, H: 0.4, Text: "{caption|Mai & Nam}", SizeIn: 0.2})
	d.Caption, d.Enabled = true, true
	saved, err := l.SaveCustom(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 || !saved.Editable || saved.Builtin {
		t.Fatalf("%+v", saved)
	}
	for _, f := range []string{"frame.json", "preview.png"} {
		if _, err := os.Stat(filepath.Join(l.Dir, "custom", saved.ID, f)); err != nil {
			t.Fatalf("%s missing: %v", f, err)
		}
	}
	// survives a restart
	again := Load(l.Dir)
	g := again.Get(saved.ID)
	if g == nil || g.Background != "#223344" || !g.Enabled || len(g.Layers) != len(d.Layers) || g.Photos() != 4 {
		t.Fatalf("%+v", g)
	}
	if len(again.Errs) != 0 {
		t.Fatal(again.Errs)
	}
	// editing bumps the revision and keeps the id
	g.Name = "Renamed"
	s2, err := again.SaveCustom(*g, nil)
	if err != nil || s2.Revision != 2 || s2.ID != saved.ID {
		t.Fatalf("%v %+v", err, s2)
	}
}

func TestBuiltInsAndReadOnlyFramesCannotBeOverwrittenOrDeleted(t *testing.T) {
	l := newLib(t)
	b := *l.Get("rose-pink.strip4")
	if _, err := l.SaveCustom(b, nil); err == nil {
		t.Fatal("a built-in frame must not be overwritten")
	}
	if l.DeleteCustom("rose-pink.strip4") == nil || l.DeleteCustom("strip-2x6-4") == nil || l.DeleteCustom("nope") == nil {
		t.Fatal("only custom frames can be deleted")
	}
	// a flat user file is read-only too
	_ = os.WriteFile(filepath.Join(l.Dir, "mine.json"), []byte(`{"id":"mine","name":"Mine","widthIn":2,"heightIn":2,"dpi":100,"slots":[{"x":0,"y":0,"w":2,"h":2}]}`), 0o644)
	l.Reload()
	m := *l.Get("mine")
	if _, err := l.SaveCustom(m, nil); err == nil || l.DeleteCustom("mine") == nil {
		t.Fatal("flat user files are read-only; duplicate to edit")
	}
	d := draftFrom(t, l, "mine", "")
	if d.ID == "mine" || d.Name != "Mine copy" {
		t.Fatalf("%+v", d)
	}
}

func TestSaveRejectsInvalidFramesAndNeverLeavesPartialFolders(t *testing.T) {
	l := newLib(t)
	d := draftFrom(t, l, "plain.strip4", "Bad")
	d.Slots[0].W = 50
	if _, err := l.SaveCustom(d, nil); err == nil {
		t.Fatal("slot outside the canvas accepted")
	}
	d = draftFrom(t, l, "plain.strip4", "Missing image")
	d.Overlay = "img-0000000000.png"
	if _, err := l.SaveCustom(d, nil); err == nil || !strings.Contains(err.Error(), "import it again") {
		t.Fatalf("a frame that refers to a missing image must not be saved: %v", err)
	}
	d = draftFrom(t, l, "plain.strip4", "")
	d.Name = "  "
	if _, err := l.SaveCustom(d, nil); err == nil {
		t.Fatal("empty name accepted")
	}
	es, _ := os.ReadDir(filepath.Join(l.Dir, "custom"))
	for _, e := range es {
		t.Errorf("leftover in custom/: %s", e.Name())
	}
}

func TestInterruptedSaveIsRepairedOnNextLoad(t *testing.T) {
	l := newLib(t)
	d := draftFrom(t, l, "plain.strip4", "Safe")
	saved, _ := l.SaveCustom(d, nil)
	root := filepath.Join(l.Dir, "custom")
	// crash after the old folder moved away but before the new one took its place
	_ = os.Rename(filepath.Join(root, saved.ID), filepath.Join(root, "."+saved.ID+".old-abcd"))
	_ = os.MkdirAll(filepath.Join(root, "."+saved.ID+".tmp-ffff", "assets"), 0o755)
	again := Load(l.Dir)
	if again.Get(saved.ID) == nil {
		t.Fatal("the previous version must be restored")
	}
	if es, _ := os.ReadDir(root); len(es) != 1 {
		t.Fatalf("temporary folders must be cleaned: %v", es)
	}
}

func TestDeleteMovesFrameAsideAndDoesNotTouchSnapshots(t *testing.T) {
	l := newLib(t)
	d := draftFrom(t, l, "plain.strip4", "Gone")
	d.Enabled = true
	saved, _ := l.SaveCustom(d, nil)
	snapDir := filepath.Join(t.TempDir(), "001")
	if _, err := l.TakeSnapshot(snapDir, saved.ID, "", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := l.DeleteCustom(saved.ID); err != nil {
		t.Fatal(err)
	}
	if l.Get(saved.ID) != nil {
		t.Fatal("still listed")
	}
	if es, _ := os.ReadDir(filepath.Join(l.Dir, "custom", ".deleted")); len(es) != 1 {
		t.Fatal("deleted frame should be kept aside for recovery")
	}
	snap, assets, err := ReadSnapshot(snapDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Compose(snap.Frame, SamplePhotos(4), assets, snap.Context()); err != nil {
		t.Fatalf("snapshot must still render: %v", err)
	}
	// the same id can be reused later; the old session keeps its own copy
	if _, err := l.SaveCustom(Template{ID: saved.ID, Name: "Again", Geometry: saved.Geometry, Style: saved.Style}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestEnableDisableCustomFrameIsStoredInItsOwnFile(t *testing.T) {
	l := newLib(t)
	saved, _ := l.SaveCustom(draftFrom(t, l, "plain.strip4", "Switch"), nil)
	if saved.Enabled {
		t.Fatal("a new custom frame starts hidden from guests until the operator enables it")
	}
	if err := l.SetEnabled(saved.ID, true); err != nil {
		t.Fatal(err)
	}
	if g := Load(l.Dir).Get(saved.ID); !g.Enabled || g.Revision != 1 {
		t.Fatalf("%+v", g)
	}
	found := false
	for _, f := range Load(l.Dir).Active() {
		found = found || f.ID == saved.ID
	}
	if !found {
		t.Fatal("enabled custom frame must be offered")
	}
}

func TestImportAcceptsPNGAndJPEGWithHashNamesAndReportsAlpha(t *testing.T) {
	st := newLib(t).Staging()
	p, err := st.Import(overlayPNG(200, 600))
	if err != nil || !p.HasAlpha || p.Format != "png" || !strings.HasSuffix(p.Name, ".png") || p.Width != 200 || p.Height != 600 {
		t.Fatalf("%v %+v", err, p)
	}
	j, err := st.Import(jpegBytes(300, 200))
	if err != nil || j.HasAlpha || j.Format != "jpeg" || !strings.HasSuffix(j.Name, ".jpg") {
		t.Fatalf("%v %+v", err, j)
	}
	if !ValidAsset(p.Name) || !ValidAsset(j.Name) {
		t.Fatal("stored names must satisfy the asset name rules")
	}
	if again, _ := st.Import(overlayPNG(200, 600)); again.Name != p.Name {
		t.Fatal("the same file must map to the same name")
	}
	opaque, _ := st.Import(pngBytes(media.Solid(20, 20, color.NRGBA{1, 2, 3, 255})))
	if opaque.HasAlpha {
		t.Fatal("an opaque PNG has no transparency")
	}
}

func TestImportRejectsHostileOrUnsupportedFiles(t *testing.T) {
	st := newLib(t).Staging()
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>fetch('http://x')</script></svg>`)
	html := []byte(`<html><img src="http://evil/x.png"></html>`)
	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	exe := append([]byte("MZ\x90\x00"), make([]byte, 100)...)
	pngWithJunk := append(overlayPNG(10, 10), []byte("<?php system($_GET[0]); ?>")...)
	for name, b := range map[string][]byte{"svg": svg, "html": html, "gif": gif, "exe": exe, "empty": nil, "truncated": overlayPNG(100, 100)[:60]} {
		if _, err := st.Import(b); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// trailing bytes after a valid PNG do not make it executable: it is only ever decoded as an image
	if info, err := st.Import(pngWithJunk); err != nil {
		t.Logf("png with trailing bytes rejected (%v)", err)
	} else if !strings.HasSuffix(info.Name, ".png") {
		t.Errorf("name must be hash.png, got %s", info.Name)
	}
	big := make([]byte, MaxAssetBytes+1)
	if _, err := st.Import(big); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("oversized file accepted: %v", err)
	}
}

func TestTransparentOverlayEndToEndThroughImportSaveAndSnapshot(t *testing.T) {
	l := newLib(t)
	info, err := l.Staging().Import(overlayPNG(600, 1800))
	if err != nil {
		t.Fatal(err)
	}
	d := draftFrom(t, l, "plain.strip4", "With overlay")
	d.Background, d.Overlay, d.Enabled = "#000000", info.Name, true
	saved, err := l.SaveCustom(d, l.Staging().FS())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(l.Dir, "custom", saved.ID, "assets", info.Name)); err != nil {
		t.Fatalf("the image must be copied into the frame: %v", err)
	}
	// the staging area can be wiped; the frame owns its copy
	l.Staging().Purge(0)
	photos := []image.Image{solid(100, 100, color.NRGBA{0, 0, 255, 255}), solid(100, 100, color.NRGBA{0, 0, 255, 255}), solid(100, 100, color.NRGBA{0, 0, 255, 255}), solid(100, 100, color.NRGBA{0, 0, 255, 255})}
	out, err := l.Compose(*l.Get(saved.ID), photos, RenderContext{})
	if err != nil {
		t.Fatal(err)
	}
	// 300 DPI canvas 600x1800: green box (opaque) top-left, red 50% band mid-height, photo visible elsewhere
	if c := out.NRGBAAt(20, 20); c.G < 250 || c.R > 5 || c.B > 5 {
		t.Fatalf("opaque overlay pixels must cover the photo: %v", c)
	}
	mid := out.NRGBAAt(300, 900+20)
	if mid.R < 110 {
		t.Fatalf("half-transparent band must tint what is below: %v", mid)
	}
	if c := out.NRGBAAt(300, 500); c.B < 250 {
		t.Fatalf("a transparent overlay area must show the photo: %v", c)
	}
	// a session snapshot carries the overlay: it still renders after the frame is deleted and the staging is gone
	snapDir := filepath.Join(t.TempDir(), "001")
	if _, err := l.TakeSnapshot(snapDir, saved.ID, "", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = l.DeleteCustom(saved.ID)
	snap, assets, err := ReadSnapshot(snapDir)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := Compose(snap.Frame, photos, assets, snap.Context())
	if err != nil {
		t.Fatal(err)
	}
	if c := out2.NRGBAAt(20, 20); c.G < 250 {
		t.Fatalf("snapshot overlay lost: %v", c)
	}
}

func TestBackgroundImageAndImageLayersAreDrawnInOrder(t *testing.T) {
	l := newLib(t)
	bg, _ := l.Staging().Import(jpegBytes(600, 1800))
	logo, _ := l.Staging().Import(pngBytes(media.Solid(100, 100, color.NRGBA{0, 0, 255, 255})))
	d := draftFrom(t, l, "plain.strip4", "Layers")
	d.BackgroundImage = bg.Name
	d.Layers = []Layer{{ID: "logo", Kind: LayerImage, Asset: logo.Name, X: 0.5, Y: 5.4, W: 0.5, H: 0.5}}
	saved, err := l.SaveCustom(d, l.Staging().FS())
	if err != nil {
		t.Fatal(err)
	}
	out, err := l.Compose(*l.Get(saved.ID), SamplePhotos(4), RenderContext{})
	if err != nil {
		t.Fatal(err)
	}
	if c := out.NRGBAAt(5, 5); c.R < 150 || c.B > 100 { // the orange JPEG background shows in the margin
		t.Fatalf("background artwork missing: %v", c)
	}
	if c := out.NRGBAAt(225, 1635); c.B < 240 || c.R > 20 { // the logo image layer is on top
		t.Fatalf("image layer missing: %v", c)
	}
}

func TestPreviewOfADraftUsesSamePipelineAndRejectsUnknownAssets(t *testing.T) {
	l := newLib(t)
	d := draftFrom(t, l, "plain.strip4", "Draft")
	d.Overlay = "img-deadbeef00.png"
	if _, err := Preview(d, Layered{l.Staging().FS()}, RenderContext{}, 200); err == nil {
		t.Fatal("a missing image must be an error, not a silently empty overlay")
	}
}
