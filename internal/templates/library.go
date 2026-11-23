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
	Enabled   map[string]bool `json:"enabled"`
	Order     []string        `json:"order,omitempty"`
	Favorites map[string]bool `json:"favorites,omitempty"`
}

// Library is the set of built-in plus user frames. It is safe for concurrent use.
type Library struct {
	Dir  string
	Errs []string // problems loading user frames, shown in the UI

	mu        sync.RWMutex
	list      []Template
	enabled   map[string]bool
	order     []string
	favorites map[string]bool
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
		if json.Unmarshal(b, &st) == nil {
			if st.Enabled != nil {
				l.enabled = st.Enabled
			}
			l.order, l.favorites = st.Order, st.Favorites
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
	list = append(list, l.loadCustom(have)...)
	for i := range list {
		if on, ok := l.enabled[list[i].ID]; ok && !list[i].Editable && !list[i].Legacy {
			list[i].Enabled = on
		}
		if list[i].Legacy {
			list[i].Order += 200000
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Order < list[j].Order })
	rank := map[string]int{}
	for i, id := range l.order {
		rank[id] = i + 1
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := rank[list[i].ID], rank[list[j].ID]
		if a == 0 {
			return false
		}
		if b == 0 {
			return true
		}
		return a < b
	})
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
	if l.list[idx].Editable { // custom frames carry their own switch
		if err := l.setCustomEnabled(&l.list[idx], on); err != nil {
			return err
		}
		l.list[idx].Enabled = on
		return nil
	}
	next := map[string]bool{}
	for k, v := range l.enabled {
		next[k] = v
	}
	next[id] = on
	b, _ := json.MarshalIndent(libState{Enabled: next, Order: l.order, Favorites: l.favorites}, "", "  ")
	if err := storage.WriteReplace(filepath.Join(l.Dir, stateFile), b); err != nil {
		return err
	}
	l.enabled = next
	l.list[idx].Enabled = on
	return nil
}

// SetCatalogue persists presentation metadata atomically; artwork and frozen
// session snapshots are never modified.
func (l *Library) SetCatalogue(order []string, favorites map[string]bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	known := map[string]bool{}
	for _, t := range l.list {
		if !t.Legacy {
			known[t.ID] = true
		}
	}
	seen := map[string]bool{}
	for _, id := range order {
		if !known[id] || seen[id] {
			return fmt.Errorf("invalid or duplicate frame %q", id)
		}
		seen[id] = true
	}
	for id := range favorites {
		if !known[id] {
			return fmt.Errorf("unknown frame %q", id)
		}
	}
	clean := map[string]bool{}
	for id, on := range favorites {
		if on {
			clean[id] = true
		}
	}
	b, err := json.MarshalIndent(libState{Enabled: l.enabled, Order: order, Favorites: clean}, "", "  ")
	if err != nil {
		return err
	}
	if err = storage.WriteReplace(filepath.Join(l.Dir, stateFile), b); err != nil {
		return err
	}
	l.order = append([]string(nil), order...)
	l.favorites = clean
	rank := map[string]int{}
	for i, id := range l.order {
		rank[id] = i + 1
	}
	sort.SliceStable(l.list, func(i, j int) bool {
		a, b := rank[l.list[i].ID], rank[l.list[j].ID]
		if a == 0 {
			return false
		}
		if b == 0 {
			return true
		}
		return a < b
	})
	return nil
}

func (l *Library) Catalogue() ([]string, map[string]bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	order := append([]string(nil), l.order...)
	favorites := map[string]bool{}
	for id, on := range l.favorites {
		favorites[id] = on
	}
	return order, favorites
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
