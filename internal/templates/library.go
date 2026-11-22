package templates

import (
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

// stateFile remembers which frames the operator has enabled or disabled
// (built-in frames are code, so their switches live next to the user's files).
const stateFile = "frame-state.json"

type libState struct {
	Enabled map[string]bool `json:"enabled"`
}

// Library is the set of built-in plus user frames. It is safe for concurrent use.
type Library struct {
	Dir  string
	Errs []string // problems loading user frames, shown in the UI

	mu      sync.RWMutex
	list    []Template
	enabled map[string]bool
}

// Load reads built-ins and user frames from dir (created if missing).
func Load(dir string) *Library {
	l := &Library{Dir: dir, enabled: map[string]bool{}}
	_ = os.MkdirAll(dir, 0o755)
	l.reload()
	return l
}

func (l *Library) reload() {
	list := Builtin()
	l.Errs = nil
	if b, err := os.ReadFile(filepath.Join(l.Dir, stateFile)); err == nil {
		var st libState
		if json.Unmarshal(b, &st) == nil && st.Enabled != nil {
			l.enabled = st.Enabled
		}
	}
	files, _ := filepath.Glob(filepath.Join(l.Dir, "*.json"))
	sort.Strings(files)
	have := map[string]bool{}
	for _, t := range list {
		have[t.ID] = true
	}
	for i, f := range files {
		if filepath.Base(f) == stateFile {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		t := Template{Enabled: true}
		if err := json.Unmarshal(b, &t); err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		if t.Kind == "" {
			t.Kind = "custom"
		}
		if t.Category == "" {
			t.Category = CatCustom
		}
		t.Builtin, t.Legacy = false, false
		if t.Revision == 0 {
			t.Revision = 1
		}
		t.Order = 100000 + i
		if err := t.Validate(); err != nil {
			l.Errs = append(l.Errs, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		if have[t.ID] {
			l.Errs = append(l.Errs, fmt.Sprintf("%s: id %q already exists", filepath.Base(f), t.ID))
			continue
		}
		have[t.ID] = true
		list = append(list, t)
	}
	for i := range list {
		if on, ok := l.enabled[list[i].ID]; ok && !list[i].Legacy {
			list[i].Enabled = on
		}
		if list[i].Legacy {
			list[i].Order += 200000
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Order < list[j].Order })
	l.list = list
}

// All returns every frame, including disabled and legacy ones.
func (l *Library) All() []Template {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Template, len(l.list))
	for i, t := range l.list {
		out[i] = t.Clone()
	}
	return out
}

// Active returns the frames guests may choose in Booth Mode.
func (l *Library) Active() []Template {
	var out []Template
	for _, t := range l.All() {
		if t.Enabled && !t.Legacy {
			out = append(out, t)
		}
	}
	return out
}

// Get returns a frame by id (nil if unknown).
func (l *Library) Get(id string) *Template {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for i := range l.list {
		if l.list[i].ID == id {
			t := l.list[i].Clone()
			return &t
		}
	}
	return nil
}

// SetEnabled switches a frame on or off in Booth Mode and persists the choice.
func (l *Library) SetEnabled(id string, on bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	idx := -1
	for i := range l.list {
		if l.list[i].ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("unknown frame %q", id)
	}
	if l.list[idx].Legacy {
		return fmt.Errorf("%q is a legacy layout and cannot be offered to guests", id)
	}
	next := map[string]bool{}
	for k, v := range l.enabled {
		next[k] = v
	}
	next[id] = on
	b, _ := json.MarshalIndent(libState{Enabled: next}, "", "  ")
	if err := storage.WriteReplace(filepath.Join(l.Dir, stateFile), b); err != nil {
		return err
	}
	l.enabled = next
	l.list[idx].Enabled = on
	return nil
}

// Assets returns the image files of frame id (nil when it has none).
func (l *Library) Assets(id string) fs.FS {
	t := l.Get(id)
	if t == nil || len(t.AssetNames()) == 0 {
		return nil
	}
	return os.DirFS(l.Dir)
}

// Compose renders photos into frame t using its assets.
func (l *Library) Compose(t Template, photos []image.Image, rc RenderContext) (*image.NRGBA, error) {
	return Compose(t, photos, l.Assets(t.ID), rc)
}

// Render composes and encodes a JPEG with the frame DPI in its header.
func Render(t Template, photos []image.Image, assets fs.FS, rc RenderContext) (data []byte, w, h int, err error) {
	img, err := Compose(t, photos, assets, rc)
	if err != nil {
		return nil, 0, 0, err
	}
	return encode(img, t.DPI)
}

// Reload re-reads user frame files (after the operator changed them on disk).
func (l *Library) Reload() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reload()
}
