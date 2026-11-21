package app

import "github.com/st-mich43l/apexvoid-photobooth/internal/settings"

// GetSettings returns the current settings.
func (s *Service) GetSettings() settings.Settings { return s.settings.Get() }

// SaveSettings validates and persists settings.
func (s *Service) SaveSettings(n settings.Settings) (SaveResult, error) {
	old := s.settings.Get()
	if err := s.settings.Save(n); err != nil {
		return SaveResult{}, newUserErr(err.Error(), "")
	}
	s.mu.Lock()
	s.encoder = "" // re-detect if ffmpeg changed
	s.mu.Unlock()
	restart := old.MediaDir != n.MediaDir
	s.emit("settings:changed", n)
	return SaveResult{RestartRequired: restart}, nil
}
