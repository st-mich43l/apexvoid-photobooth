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

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
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

// CameraMode is a remembered capture mode for one camera source.
type CameraMode struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	FPS    int    `json:"fps"`
	Format string `json:"format"`
}

// Settings is the persisted configuration.
type Settings struct {
	Language         string `json:"language"`
	GuestLanguage    string `json:"guestLanguage"`
	MediaDir         string `json:"mediaDir"` // empty = default data directory
	CountdownSeconds int    `json:"countdownSeconds"`
	Shots            int    `json:"shots"`      // legacy: the frame now decides the photo count (1, 2, 3, 4 or 6)
	TemplateID       string `json:"templateId"` // the frame preselected on the frame-selection screen
	// OutputDPI is the print resolution of built-in frames: 300 (standard) or 600 (high). Custom frames keep the DPI set in the Frame Builder.
	OutputDPI int `json:"outputDpi"`
	// PreviewMirror flips the live preview horizontally (like a mirror) for guests. It is display only: photos are never mirrored.
	PreviewMirror    bool   `json:"previewMirror"`
	AutoReconnect    bool   `json:"autoReconnect"`
	EnableMock       bool   `json:"enableMock"`  // development: expose the simulated camera
	FFmpegPath       string `json:"ffmpegPath"`  // empty = beside the exe, then PATH
	CanonSDKDir      string `json:"canonSdkDir"` // empty = beside the exe, ./sdk/canon, PATH
	PrinterName      string `json:"printerName"`
	PrintPaper       string `json:"printPaper"` // default paper size name (as reported by the printer)
	PrintOrientation string `json:"printOrientation"`
	PrintFit         string `json:"printFit"`
	PrintCopies      int    `json:"printCopies"`
	PrintTwoUp       string `json:"printTwoUp"` // auto | on | off
	// Preferred sources, separate for Photo and Video mode. Restored at start-up;
	// when one is missing the operator is asked before another source is used.
	PhotoCamera camera.Preferred `json:"photoCamera"`
	VideoCamera camera.Preferred `json:"videoCamera"`
	// CameraModes remembers the chosen resolution/fps per camera source id.
	CameraModes map[string]CameraMode `json:"cameraModes"`
	// KioskPin is "pbkdf2$iterations$salt$hash" ("" = kiosk protection off). It is never sent to the UI.
	KioskPin    string `json:"kioskPin"`
	KioskPinSet bool   `json:"kioskPinSet"` // computed for the UI; ignored when saving
	Video       Video  `json:"video"`
}

// Default returns factory settings.
func Default() Settings {
	return Settings{
		Language: "en", GuestLanguage: "en", CountdownSeconds: 3, Shots: 4, TemplateID: "plain.strip4", OutputDPI: 300, AutoReconnect: true,
		PrintOrientation: "auto", PrintFit: "fit", PrintCopies: 1, PrintTwoUp: "auto",
		Video: Video{Width: 1280, Height: 720, FPS: 30, MaxSeconds: 30},
	}
}

// Validate normalises and checks values.
func (s *Settings) Validate() error {
	if s.Language == "" {
		s.Language = "en"
	}
	if s.Language != "en" && s.Language != "vi" {
		return errors.New("unsupported language")
	}
	if s.GuestLanguage == "" {
		s.GuestLanguage = "en"
	}
	if s.GuestLanguage != "en" && s.GuestLanguage != "vi" {
		return errors.New("unsupported guest language")
	}
	if s.PrintOrientation == "" {
		s.PrintOrientation = "auto"
	}
	if s.PrintFit == "" {
		s.PrintFit = "fit"
	}
	if s.PrintCopies == 0 {
		s.PrintCopies = 1
	}
	if s.PrintTwoUp == "" {
		s.PrintTwoUp = "auto"
	}
	if s.PrintOrientation != "auto" && s.PrintOrientation != "portrait" && s.PrintOrientation != "landscape" {
		return errors.New("invalid default print orientation")
	}
	if s.PrintFit != "fit" && s.PrintFit != "fill" && s.PrintFit != "actual" {
		return errors.New("invalid default print placement")
	}
	if s.PrintCopies < 1 || s.PrintCopies > 10 {
		return errors.New("default print copies must be 1–10")
	}
	if s.PrintTwoUp != "auto" && s.PrintTwoUp != "on" && s.PrintTwoUp != "off" {
		return errors.New("invalid default two-up setting")
	}
	if s.OutputDPI == 0 { // settings written before this option existed
		s.OutputDPI = 300
	}
	if s.OutputDPI != 300 && s.OutputDPI != 600 {
		return errors.New("print resolution must be 300 or 600 dpi")
	}
	if s.CountdownSeconds < 1 || s.CountdownSeconds > 10 {
		return errors.New("countdown must be 1-10 seconds")
	}
	switch s.Shots {
	case 1, 2, 3, 4, 6:
	default:
		return errors.New("number of photographs must be 1, 2, 3, 4 or 6")
	}
	if s.Video.Width < 160 || s.Video.Height < 120 || s.Video.FPS < 1 || s.Video.FPS > 120 {
		return errors.New("invalid video resolution or frame rate")
	}
	if s.Video.MaxSeconds < 1 || s.Video.MaxSeconds > 3600 {
		return errors.New("recording duration must be 1-3600 seconds")
	}
	for id, m := range s.CameraModes {
		if m.Width < 160 || m.Height < 120 || m.FPS < 1 || m.FPS > 120 {
			return fmt.Errorf("invalid capture mode for camera %q", id)
		}
	}
	if s.MediaDir != "" && !filepath.IsAbs(s.MediaDir) {
		return errors.New("media folder must be an absolute path")
	}
	return nil
}

// defaultDataDirOther is the library location on macOS/Linux (Windows uses
// <Documents>\apexvoid-studio, see library.go).
func defaultDataDirOther() string {
	switch runtime.GOOS {
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
