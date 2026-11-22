package app

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

// publicSettings is what the UI may see: the PIN record is replaced by a flag.
func (s *Service) publicSettings() settings.Settings {
	c := s.settings.Get()
	c.KioskPinSet = c.KioskPin != ""
	c.KioskPin = ""
	return c
}

// GetSettings returns the current settings (without the PIN record).
func (s *Service) GetSettings() settings.Settings { return s.publicSettings() }

// SaveSettings validates and persists settings. The PIN is managed only
// through SetKioskPin, so whatever the UI sends for it is ignored.
func (s *Service) SaveSettings(n settings.Settings) (SaveResult, error) {
	old := s.settings.Get()
	n.KioskPin, n.KioskPinSet = old.KioskPin, false
	if err := s.settings.Save(n); err != nil {
		return SaveResult{}, newUserErr(err.Error(), "")
	}
	s.mu.Lock()
	s.encoder = "" // re-detect if ffmpeg changed
	s.mu.Unlock()
	restart := old.MediaDir != n.MediaDir
	s.emit("settings:changed", s.publicSettings())
	return SaveResult{RestartRequired: restart}, nil
}

// PatchSettings changes only the named settings and leaves every other value as it
// is *now* on the backend. The UI used to send its whole copy of the settings, so a
// stale copy (for example one that predates a camera chosen on another page) silently
// overwrote newer values. The PIN is never writable here.
func (s *Service) PatchSettings(patch map[string]any) (SaveResult, error) {
	s.patchMu.Lock()
	defer s.patchMu.Unlock()
	old := s.settings.Get()
	raw, err := json.Marshal(old)
	if err != nil {
		return SaveResult{}, newUserErr(err.Error(), "")
	}
	var cur map[string]any
	if err := json.Unmarshal(raw, &cur); err != nil {
		return SaveResult{}, newUserErr(err.Error(), "")
	}
	for k, v := range patch {
		switch k {
		case "kioskPin", "kioskPinSet":
			continue
		}
		cur[k] = v
	}
	merged, err := json.Marshal(cur)
	if err != nil {
		return SaveResult{}, newUserErr(err.Error(), "")
	}
	var n settings.Settings
	if err := json.Unmarshal(merged, &n); err != nil {
		return SaveResult{}, newUserErr("Invalid setting: "+err.Error(), "")
	}
	res, err := s.SaveSettings(n)
	if err != nil {
		return res, err
	}
	pub := s.publicSettings()
	res.Settings = &pub
	return res, nil
}

// SetKioskPin sets, changes or clears (next == "") the operator PIN that
// protects leaving Booth Mode. Changing or clearing an existing PIN requires
// the current one.
func (s *Service) SetKioskPin(current, next string) error {
	cur := s.settings.Get()
	if cur.KioskPin != "" {
		if err := s.VerifyKioskPin(current); err != nil {
			return err
		}
	}
	if next == "" {
		cur.KioskPin = ""
	} else {
		h, err := settings.HashPIN(next)
		if err != nil {
			return newUserErr(err.Error(), "")
		}
		cur.KioskPin = h
	}
	if err := s.settings.Save(cur); err != nil {
		return newUserErr(err.Error(), "")
	}
	s.emit("settings:changed", s.publicSettings())
	return nil
}

const (
	pinMaxFails = 5
	pinLockout  = 30 * time.Second
)

// VerifyKioskPin checks the operator PIN. After five wrong attempts it locks
// for 30 s so a guest cannot guess it at the touchscreen.
func (s *Service) VerifyKioskPin(pin string) error {
	s.pinMu.Lock()
	defer s.pinMu.Unlock()
	if wait := time.Until(s.pinLockedUntil); wait > 0 {
		return newUserErr(fmt.Sprintf("Too many wrong PINs. Try again in %d seconds.", int(wait.Seconds())+1), "")
	}
	rec := s.settings.Get().KioskPin
	if rec == "" {
		return nil // protection is off
	}
	if settings.CheckPIN(rec, pin) {
		s.pinFails = 0
		return nil
	}
	s.pinFails++
	if s.pinFails >= pinMaxFails {
		s.pinFails, s.pinLockedUntil = 0, time.Now().Add(pinLockout)
		return newUserErr("Too many wrong PINs. Locked for 30 seconds.", "")
	}
	return newUserErr("Wrong PIN.", "")
}

// guardWrite stops operations that would write to the library while it is
// being copied, or after it was copied but the app still uses the old folder.
func (s *Service) guardWrite() error {
	if s.migrating.Load() {
		return newUserErr("The library is being moved.", "Wait until the migration finishes.")
	}
	if s.restartRequired.Load() {
		return newUserErr("The library was moved to its new folder.", "Close and reopen ApexVoid Photobooth to continue; new photos would otherwise go to the old folder.")
	}
	return nil
}
