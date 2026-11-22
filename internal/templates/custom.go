package templates

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

// Custom frames live in <templates>/custom/<id>/{frame.json, preview.png, assets/}.
// Saving is atomic (a complete new folder replaces the old one) and recoverable
// (an interrupted swap is repaired the next time the library loads).
const (
	customDir   = "custom"
	deletedDir  = ".deleted"
	stagingDir  = ".staging"
	frameFile   = "frame.json"
	previewFile = "preview.png"
)

func (l *Library) customRoot() string { return filepath.Join(l.Dir, customDir) }
func (l *Library) frameDir(id string) string {
	return filepath.Join(l.customRoot(), id)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// loadCustom reads <templates>/custom/*. It first repairs an interrupted save.
func (l *Library) loadCustom(have map[string]bool) []Template {
	root := l.customRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	// repair: "<id>.old-*" without "<id>" means a swap was interrupted after the old folder moved away
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() && strings.HasPrefix(n, ".") && strings.Contains(n, ".old-") {
			id := strings.TrimPrefix(n[:strings.Index(n, ".old-")], ".")
			if _, err := os.Stat(filepath.Join(root, id)); err != nil {
				_ = os.Rename(filepath.Join(root, n), filepath.Join(root, id))
			} else {
				_ = os.RemoveAll(filepath.Join(root, n))
			}
		} else if e.IsDir() && strings.HasPrefix(n, ".") && strings.Contains(n, ".tmp-") {
			_ = os.RemoveAll(filepath.Join(root, n)) // an unfinished save: never published
		}
	}
	entries, _ = os.ReadDir(root)
	var out []Template
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(root, e.Name(), frameFile)
		b, err := os.ReadFile(path)
		if err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("custom/%s: %v", e.Name(), err))
			continue
		}
		t := Template{Enabled: true}
		if err := json.Unmarshal(b, &t); err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("custom/%s: %v", e.Name(), err))
			continue
		}
		t.Builtin, t.Legacy, t.Editable = false, false, true
		if t.Kind == "" {
			t.Kind = "custom"
		}
		if t.Category == "" {
			t.Category = CatCustom
		}
		if t.Revision == 0 {
			t.Revision = 1
		}
		if t.ID != e.Name() {
			l.Errs = append(l.Errs, fmt.Sprintf("custom/%s: the folder name must equal the frame id %q", e.Name(), t.ID))
			continue
		}
		if err := t.Validate(); err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("custom/%s: %v", e.Name(), err))
			continue
		}
		if have[t.ID] {
			l.Errs = append(l.Errs, fmt.Sprintf("custom/%s: id %q already exists", e.Name(), t.ID))
			continue
		}
		have[t.ID] = true
		t.Order = 50000 + len(out)
		out = append(out, t)
	}
	return out
}

// AssetFS returns the image files of frame id: a custom frame's own assets or,
// for flat user files, the templates folder (legacy overlays).
func (l *Library) assetFS(t *Template) fs.FS {
	switch {
	case t == nil || t.Builtin:
		return nil
	case t.Editable:
		return os.DirFS(filepath.Join(l.frameDir(t.ID), "assets"))
	}
	return os.DirFS(l.Dir)
}

// Assets returns the image files of frame id (nil when it has none).
func (l *Library) Assets(id string) fs.FS {
	t := l.Get(id)
	if t == nil || len(t.AssetNames()) == 0 {
		return nil
	}
	return l.assetFS(t)
}

// Slug turns a display name into a frame id fragment.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		s = "frame"
	}
	return s
}

// UniqueID returns an id based on name that no frame (built-in, user or deleted-but-listed) uses.
func (l *Library) UniqueID(name string) string {
	base := "custom-" + Slug(name)
	id := base
	for i := 2; l.Get(id) != nil; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

// NewDraft returns an unsaved editable copy of frame srcID (built-in or custom)
// with a fresh id and name. It is how "create from a built-in layout" and
// "duplicate" work. Nothing is written until SaveCustom.
func (l *Library) NewDraft(srcID, name string) (Template, error) {
	src := l.Get(srcID)
	if src == nil {
		return Template{}, fmt.Errorf("frame %q does not exist", srcID)
	}
	d := src.Clone()
	if name == "" {
		name = src.Name + " copy"
	}
	d.ID, d.Name = l.UniqueID(name), name
	d.Builtin, d.Legacy, d.Editable = false, false, true
	d.Kind, d.Category, d.Revision, d.Order = "custom", CatCustom, 0, 0
	d.Design, d.LayoutID, d.VariantID = "", "", ""
	d.Enabled = false
	d.Description = ""
	return d, nil
}

// SaveCustom stores a custom frame (new or edited). staged supplies images
// imported but not yet part of a saved frame; images the frame already owns are
// kept. The write is atomic: readers never see a half-saved frame.
func (l *Library) SaveCustom(t Template, staged fs.FS) (Template, error) {
	t.Builtin, t.Legacy, t.Editable, t.VariantID = false, false, true, ""
	if t.Kind == "" {
		t.Kind = "custom"
	}
	if t.Category == "" {
		t.Category = CatCustom
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || len(t.Name) > 60 {
		return Template{}, errors.New("give the frame a name (up to 60 characters)")
	}
	if len(t.Description) > 300 {
		return Template{}, errors.New("the description is too long (max 300 characters)")
	}
	if err := t.Validate(); err != nil {
		return Template{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var existing *Template
	for i := range l.list {
		if l.list[i].ID == t.ID {
			existing = &l.list[i]
		}
	}
	if existing != nil && !existing.Editable {
		return Template{}, fmt.Errorf("%q is a built-in or read-only frame; duplicate it to make an editable copy", t.ID)
	}
	t.Revision = 1
	var ownAssets fs.FS
	if existing != nil {
		t.Revision = existing.Revision + 1
		ownAssets = os.DirFS(filepath.Join(l.frameDir(t.ID), "assets"))
	}
	if err := os.MkdirAll(l.customRoot(), 0o755); err != nil {
		return Template{}, err
	}
	tmp := filepath.Join(l.customRoot(), "."+t.ID+".tmp-"+randHex(4))
	fail := func(err error) (Template, error) { _ = os.RemoveAll(tmp); return Template{}, err }
	if err := os.MkdirAll(filepath.Join(tmp, "assets"), 0o755); err != nil {
		return fail(err)
	}
	for _, name := range t.AssetNames() {
		err := errors.New("missing")
		for _, src := range []fs.FS{staged, ownAssets} {
			if src == nil {
				continue
			}
			if err = copyAsset(src, name, filepath.Join(tmp, "assets", name)); err == nil {
				break
			}
		}
		if err != nil {
			return fail(fmt.Errorf("image %q is missing — import it again", name))
		}
	}
	// the preview image proves the frame renders with its images before it is published
	prev, err := Preview(t, os.DirFS(filepath.Join(tmp, "assets")), RenderContext{Date: time.Now()}, 360)
	if err != nil {
		return fail(fmt.Errorf("the frame cannot be drawn: %w", err))
	}
	var pb bytes.Buffer
	if err := png.Encode(&pb, prev); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, previewFile), pb.Bytes(), 0o644); err != nil {
		return fail(err)
	}
	b, _ := json.MarshalIndent(t, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, frameFile), b, 0o644); err != nil {
		return fail(err)
	}
	dir := l.frameDir(t.ID)
	old := ""
	if _, err := os.Stat(dir); err == nil {
		old = filepath.Join(l.customRoot(), "."+t.ID+".old-"+randHex(4))
		if err := os.Rename(dir, old); err != nil {
			return fail(err)
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		if old != "" {
			_ = os.Rename(old, dir)
		}
		return fail(err)
	}
	if old != "" {
		_ = os.RemoveAll(old)
	}
	l.reload()
	for i := range l.list {
		if l.list[i].ID == t.ID {
			return l.list[i].Clone(), nil
		}
	}
	return Template{}, errors.New("the frame was saved but could not be loaded again")
}

// setCustomEnabled rewrites frame.json with a new enabled flag (revision unchanged).
func (l *Library) setCustomEnabled(t *Template, on bool) error {
	c := t.Clone()
	c.Enabled = on
	b, _ := json.MarshalIndent(c, "", "  ")
	return storage.WriteReplace(filepath.Join(l.frameDir(t.ID), frameFile), b)
}

// DeleteCustom removes a custom frame from the library. The folder is moved to
// templates/custom/.deleted so it can still be recovered by hand; sessions are
// unaffected because each keeps its own snapshot.
func (l *Library) DeleteCustom(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var t *Template
	for i := range l.list {
		if l.list[i].ID == id {
			t = &l.list[i]
		}
	}
	if t == nil {
		return fmt.Errorf("unknown frame %q", id)
	}
	if !t.Editable {
		return fmt.Errorf("%q is built in or read-only and cannot be deleted", id)
	}
	dst := filepath.Join(l.customRoot(), deletedDir)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if err := os.Rename(l.frameDir(id), filepath.Join(dst, fmt.Sprintf("%s-%d", id, time.Now().Unix()))); err != nil {
		return err
	}
	l.reload()
	return nil
}

// ---- image import ----

// AssetInfo describes an imported image.
type AssetInfo struct {
	Name     string `json:"name"` // file name to store in the frame (hash-based, never the user's name)
	Format   string `json:"format"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	HasAlpha bool   `json:"hasAlpha"` // some pixels are (semi-)transparent
	Bytes    int    `json:"bytes"`
}

// Staging holds imported images until the frame that uses them is saved.
type Staging struct{ dir string }

// Staging returns the library's staging area (created on first use).
func (l *Library) Staging() *Staging { return &Staging{dir: filepath.Join(l.Dir, stagingDir)} }

// FS is the staged files.
func (s *Staging) FS() fs.FS { return os.DirFS(s.dir) }

// Import validates and stores an image. The bytes are decoded (so the file
// signature, not its name or MIME type, decides), checked against the size,
// dimension and pixel limits, and stored under a hash-based name with the
// extension of the detected format. Nothing about the original path is kept.
func (s *Staging) Import(data []byte) (AssetInfo, error) {
	if len(data) == 0 {
		return AssetInfo{}, errors.New("the file is empty")
	}
	if len(data) > MaxAssetBytes {
		return AssetInfo{}, fmt.Errorf("the file is larger than %d MB", MaxAssetBytes>>20)
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return AssetInfo{}, errors.New("the file is not a valid PNG or JPEG image (SVG and other formats are not accepted)")
	}
	img, err := DecodeImage(data)
	if err != nil {
		return AssetInfo{}, err
	}
	ext := map[string]string{"png": "png", "jpeg": "jpg"}[format]
	sum := sha256.Sum256(data)
	name := "img-" + hex.EncodeToString(sum[:5]) + "." + ext
	b := img.Bounds()
	info := AssetInfo{Name: name, Format: format, Width: b.Dx(), Height: b.Dy(), Bytes: len(data), HasAlpha: hasAlpha(img)}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return AssetInfo{}, err
	}
	if err := storage.WriteReplace(filepath.Join(s.dir, name), data); err != nil {
		return AssetInfo{}, err
	}
	return info, nil
}

func hasAlpha(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	return false
}

// Purge deletes staged files older than age.
func (s *Staging) Purge(age time.Duration) {
	es, _ := os.ReadDir(s.dir)
	for _, e := range es {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > age {
			_ = os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
}

// Layered tries each file system in turn.
type Layered []fs.FS

// Open implements fs.FS.
func (ls Layered) Open(name string) (fs.File, error) {
	for _, f := range ls {
		if f == nil {
			continue
		}
		if file, err := f.Open(name); err == nil {
			return file, nil
		}
	}
	return nil, fs.ErrNotExist
}

var _ io.Reader = (*bytes.Reader)(nil)
