package templates

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Snapshot is an immutable copy of a frame (metadata, the chosen colour
// variant and every image it refers to) taken for one session. Later edits or
// deletion of the frame never change what a session renders or reprints.
type Snapshot struct {
	Version   int      `json:"version"`
	FrameID   string   `json:"frameId"`
	VariantID string   `json:"variantId"`
	Name      string   `json:"name"`
	Revision  int      `json:"revision"`
	Caption   string   `json:"caption"`
	Date      string   `json:"date"` // RFC 3339; the session date shown by {date}
	TakenAt   string   `json:"takenAt"`
	Frame     Template `json:"frame"` // resolved: the variant is already applied
}

// Context is what text layers show.
func (s Snapshot) Context() RenderContext {
	rc := RenderContext{Caption: s.Caption}
	if t, err := time.Parse(time.RFC3339, s.Date); err == nil {
		rc.Date = t
	}
	return rc
}

const snapshotFile = "frame.json"

// TakeSnapshot freezes frame id with the colour variant into dir (which must
// not exist yet). It is atomic: a half-written snapshot is never visible.
func (l *Library) TakeSnapshot(dir, id, variantID, caption string, date time.Time) (Snapshot, error) {
	base := l.Get(id)
	if base == nil {
		return Snapshot{}, fmt.Errorf("frame %q does not exist", id)
	}
	resolved, err := base.WithVariant(variantID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := resolved.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("frame %q is not usable: %w", id, err)
	}
	if !base.Caption {
		caption = ""
	}
	if len(caption) > 40 {
		caption = string([]rune(caption)[:40])
	}
	snap := Snapshot{Version: 1, FrameID: base.ID, VariantID: variantID, Name: base.Name, Revision: base.Revision, Caption: caption, Frame: resolved,
		TakenAt: time.Now().UTC().Format(time.RFC3339)}
	if !date.IsZero() {
		snap.Date = date.Format(time.RFC3339)
	}
	if _, err := os.Lstat(dir); err == nil {
		return Snapshot{}, fmt.Errorf("snapshot %s already exists", dir)
	}
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "assets"), 0o755); err != nil {
		return Snapshot{}, err
	}
	fail := func(err error) (Snapshot, error) { _ = os.RemoveAll(tmp); return Snapshot{}, err }
	src := l.Assets(id)
	for _, name := range resolved.AssetNames() {
		if err := copyAsset(src, name, filepath.Join(tmp, "assets", name)); err != nil {
			return fail(fmt.Errorf("frame image %q: %w", name, err))
		}
	}
	b, _ := json.MarshalIndent(snap, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, snapshotFile), b, 0o644); err != nil {
		return fail(err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		return fail(err)
	}
	return snap, nil
}

func copyAsset(src fs.FS, name, dst string) error {
	if src == nil || !ValidAsset(name) {
		return errors.New("is missing")
	}
	in, err := src.Open(name)
	if err != nil {
		return errors.New("is missing")
	}
	defer in.Close()
	data, err := io.ReadAll(io.LimitReader(in, MaxAssetBytes+1))
	if err != nil {
		return err
	}
	if len(data) > MaxAssetBytes {
		return errors.New("is too large")
	}
	return os.WriteFile(dst, data, 0o644)
}

// ReadSnapshot loads a snapshot and the assets it carries.
func ReadSnapshot(dir string) (Snapshot, fs.FS, error) {
	b, err := os.ReadFile(filepath.Join(dir, snapshotFile))
	if err != nil {
		return Snapshot{}, nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return Snapshot{}, nil, fmt.Errorf("frame snapshot is damaged: %w", err)
	}
	if err := s.Frame.Validate(); err != nil {
		return Snapshot{}, nil, fmt.Errorf("frame snapshot is invalid: %w", err)
	}
	return s, os.DirFS(filepath.Join(dir, "assets")), nil
}
