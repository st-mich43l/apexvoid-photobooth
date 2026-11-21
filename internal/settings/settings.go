// Package settings loads and saves the application configuration as a small
// JSON file in the per-user config directory.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

const AppDirName = "ApexVoidPhotobooth"

// Video holds recording preferences.
type Video struct {
	Device     string `json:"device"` // video device name as reported by the platform
	AudioInput string `json:"audioInput"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FPS        int    `json:"fps"`
	MaxSeconds int    `json:"maxSeconds"`
}

// Settings is the persisted configuration.
type Settings struct {
	MediaDir         string `json:"mediaDir"` // empty = default data directory
	CountdownSeconds int    `json:"countdownSeconds"`
	Shots            int    `json:"shots"` // 1, 2, 4 or 6
	TemplateID       string `json:"templateId"`
	AutoReconnect    bool   `json:"autoReconnect"`
	EnableMock       bool   `json:"enableMock"`  // development: expose the simulated camera
	FFmpegPath       string `json:"ffmpegPath"`  // empty = beside the exe, then PATH
	CanonSDKDir      string `json:"canonSdkDir"` // empty = beside the exe, ./sdk/canon, PATH
	PrinterName      string `json:"printerName"`
	Video            Video  `json:"video"`
}

// Default returns factory settings.
func Default() Settings {
	return Settings{
		CountdownSeconds: 3, Shots: 4, TemplateID: "strip-2x6-4", AutoReconnect: true,
		Video: Video{Width: 1280, Height: 720, FPS: 30, MaxSeconds: 30},
	}
}

// Validate normalises and checks values.
func (s *Settings) Validate() error {
	if s.CountdownSeconds < 1 || s.CountdownSeconds > 10 {
		return errors.New("countdown must be 1-10 seconds")
	}
	switch s.Shots {
	case 1, 2, 4, 6:
	default:
		return errors.New("number of photographs must be 1, 2, 4 or 6")
	}
	if s.Video.Width < 160 || s.Video.Height < 120 || s.Video.FPS < 1 || s.Video.FPS > 120 {
		return errors.New("invalid video resolution or frame rate")
	}
	if s.Video.MaxSeconds < 1 || s.Video.MaxSeconds > 3600 {
		return errors.New("recording duration must be 1-3600 seconds")
	}
	if s.MediaDir != "" && !filepath.IsAbs(s.MediaDir) {
		return errors.New("media folder must be an absolute path")
	}
	return nil
}

// DefaultDataDir is where the media library lives unless configured.
func DefaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, AppDirName)
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, "Library", "Application Support", AppDirName)
		}
	default:
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, AppDirName)
		}
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".local", "share", AppDirName)
		}
	}
	return filepath.Join(os.TempDir(), AppDirName)
}

// Manager persists Settings at Path.
type Manager struct {
	Path string
	mu   sync.RWMutex
	cur  Settings
}

// DefaultPath is <user config dir>/ApexVoidPhotobooth/settings.json.
func DefaultPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, AppDirName, "settings.json")
}

// Load reads settings; a missing file yields defaults, a corrupt file is
// moved aside (settings.json.corrupt) and defaults are used so the app still
// starts.
func Load(path string) (*Manager, error) {
	m := &Manager{Path: path, cur: Default()}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	s := Default()
	if err := json.Unmarshal(b, &s); err != nil || s.Validate() != nil {
		_ = os.Rename(path, path+".corrupt")
		return m, nil
	}
	m.cur = s
	return m, nil
}

// Get returns a copy of the current settings.
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cur
}

// Save validates and persists s.
func (m *Manager) Save(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := storage.WriteReplace(m.Path, b); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	m.mu.Lock()
	m.cur = s
	m.mu.Unlock()
	return nil
}

// DataDir returns the effective media library root.
func (m *Manager) DataDir() string {
	if d := m.Get().MediaDir; d != "" {
		return d
	}
	return DefaultDataDir()
}
