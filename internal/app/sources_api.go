package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

// Modes a source can be preferred for.
const (
	ModePhoto = "photo"
	ModeVideo = "video"
)

// CameraSources is the Camera Sources screen's data.
type CameraSources struct {
	Devices        []camera.DeviceInfo `json:"devices"`
	ActiveID       string              `json:"activeId"`
	PhotoPreferred camera.Preferred    `json:"photoPreferred"`
	VideoPreferred camera.Preferred    `json:"videoPreferred"`
	PhotoMatchID   string              `json:"photoMatchId"` // discovered device that satisfies the photo preference ("" = not found)
	VideoMatchID   string              `json:"videoMatchId"`
	RestoreNote    string              `json:"restoreNote"` // why start-up did not connect the preferred camera
	FFmpegFound    bool                `json:"ffmpegFound"`
}

// suitable reports whether a source can serve a mode: photos need a still-capable
// backend; video needs a streamed (UVC) source, never the simulated or EDSDK one.
func suitable(d camera.DeviceInfo, mode string) bool {
	if d.DuplicateOf != "" {
		return false
	}
	switch mode {
	case ModeVideo:
		return d.Driver == "uvc"
	default:
		return true
	}
}

// preferenceRank orders fallback candidates: Canon SDK first for photos, then
// real webcams, then others; simulated cameras last.
func preferenceRank(d camera.DeviceInfo, mode string) int {
	switch {
	case d.Simulated:
		return 9
	case mode == ModePhoto && d.Driver == "canon-edsdk":
		return 0
	case d.Kind == camera.KindWebcam:
		return 1
	case d.Kind == camera.KindCaptureCard, d.Kind == camera.KindCanonUVC:
		return 2
	case d.Kind == camera.KindVirtual:
		return 3
	}
	return 4
}

func (s *Service) pref(mode string) camera.Preferred {
	st := s.settings.Get()
	if mode == ModeVideo {
		return st.VideoCamera
	}
	return st.PhotoCamera
}

// ListCameraSources discovers every source (refreshable at any time, no restart).
func (s *Service) ListCameraSources() (CameraSources, error) {
	devs, err := s.ListCameras()
	out := CameraSources{Devices: devs, PhotoPreferred: s.pref(ModePhoto), VideoPreferred: s.pref(ModeVideo), RestoreNote: s.restoreNote()}
	if st := s.mgr.Status(); st.Device != nil && !st.Reconnecting {
		out.ActiveID = st.Device.ID
	}
	if d, ok := out.PhotoPreferred.Match(devs); ok {
		out.PhotoMatchID = d.ID
	}
	if d, ok := out.VideoPreferred.Match(devs); ok {
		out.VideoMatchID = d.ID
	}
	_, ferr := video.Locate(s.settings.Get().FFmpegPath)
	out.FFmpegFound = ferr == nil
	return out, err
}

func (s *Service) restoreNote() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restoreMsg
}

func (s *Service) setRestoreNote(m string) {
	s.mu.Lock()
	s.restoreMsg = m
	s.mu.Unlock()
}

func (s *Service) discoverFresh() ([]camera.DeviceInfo, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.mgr.Discover(ctx)
}

// SetPreferredCamera remembers a source for Photo or Video mode.
func (s *Service) SetPreferredCamera(mode, id string) error {
	if mode != ModePhoto && mode != ModeVideo {
		return newUserErr("Unknown mode.", "")
	}
	devs, err := s.discoverFresh()
	if err != nil {
		return userErr(err)
	}
	for _, d := range devs {
		if d.ID != id {
			continue
		}
		if !suitable(d, mode) {
			if d.DuplicateOf != "" {
				return newUserErr("That entry is the same physical camera as another listed source.", "Choose the other entry.")
			}
			return newUserErr("This source cannot record video.", "Video needs a webcam/UVC source (a Canon body in USB streaming mode works). The Canon SDK source is for photos.")
		}
		cur := s.settings.Get()
		p := camera.Preferred{ID: d.ID, Name: d.Name, Driver: d.Driver, USBID: d.USBID}
		if mode == ModeVideo {
			cur.VideoCamera = p
		} else {
			cur.PhotoCamera = p
		}
		if err := s.settings.Save(cur); err != nil {
			return newUserErr(err.Error(), "")
		}
		s.emit("settings:changed", s.publicSettings())
		return nil
	}
	return newUserErr("That camera is no longer available.", "Refresh the list and choose again.")
}

// ClearPreferredCamera forgets the preference for a mode.
func (s *Service) ClearPreferredCamera(mode string) error {
	cur := s.settings.Get()
	switch mode {
	case ModePhoto:
		cur.PhotoCamera = camera.Preferred{}
	case ModeVideo:
		cur.VideoCamera = camera.Preferred{}
	default:
		return newUserErr("Unknown mode.", "")
	}
	if err := s.settings.Save(cur); err != nil {
		return newUserErr(err.Error(), "")
	}
	s.emit("settings:changed", s.publicSettings())
	return nil
}

// cameraPrefs gives the UVC driver the mode remembered for a device, falling
// back to the global video defaults.
func (s *Service) cameraPrefs(deviceID string) (w, h, fps int) {
	st := s.settings.Get()
	if m, ok := st.CameraModes[deviceID]; ok {
		return m.Width, m.Height, m.FPS
	}
	return st.Video.Width, st.Video.Height, st.Video.FPS
}

// SetCameraMode remembers the resolution/frame rate for a source. If that
// source is the connected one, it is reconnected so the new mode applies.
func (s *Service) SetCameraMode(id string, m settings.CameraMode) error {
	if err := s.guardOperation(); err != nil {
		return err
	}
	cur := s.settings.Get()
	next := map[string]settings.CameraMode{}
	for k, v := range cur.CameraModes {
		next[k] = v
	}
	next[id] = m
	cur.CameraModes = next
	if err := s.settings.Save(cur); err != nil {
		return newUserErr(err.Error(), "")
	}
	s.emit("settings:changed", s.publicSettings())
	if st := s.mgr.Status(); st.Device != nil && st.Device.ID == id {
		return s.switchTo(st.Device.ID, true)
	}
	return nil
}

// guardOperation refuses a camera change while a capture, session or
// recording is running, so a device is never swapped under an operation.
func (s *Service) guardOperation() error {
	switch s.eng.Snapshot().State {
	case "COUNTDOWN", "CAPTURING", "REVIEW", "SAVING":
		return newUserErr("A photo session is running.", "Finish or cancel it before changing the camera.")
	}
	if v := s.VideoState().State; v == "recording" || v == "saving" {
		return newUserErr("A video is being recorded.", "Stop the recording before changing the camera.")
	}
	if s.migrating.Load() {
		return newUserErr("The library is being moved.", "Wait until the migration finishes.")
	}
	return nil
}

// switchTo is the single, safe way to change the connected camera:
// validate → stop preview → release the previous device (and its FFmpeg
// process) → connect → restart the preview if it was running.
func (s *Service) switchTo(id string, reconnectSame bool) error {
	if err := s.guardOperation(); err != nil {
		return err
	}
	devs, err := s.discoverFresh()
	if err != nil {
		return userErr(err)
	}
	var target *camera.DeviceInfo
	for i := range devs {
		if devs[i].ID == id {
			target = &devs[i]
		}
	}
	if target == nil {
		return newUserErr("That camera is no longer available.", "Press Refresh and choose it again.")
	}
	st := s.mgr.Status()
	if st.Device != nil && st.Device.ID == id && !reconnectSame && !st.Reconnecting && st.State != camera.StateError {
		return nil // already connected
	}
	wasLive := st.LiveView
	_ = s.eng.Reset() // stops the preview owned by the shooting engine
	ctx, cancel := s.ctx()
	defer cancel()
	if err := s.mgr.Connect(ctx, *target); err != nil {
		s.log.Warn("connect failed", "device", target.Name, "err", err)
		return userErr(err)
	}
	s.log.Info("camera connected", "device", target.Name, "backend", target.Driver, "id", target.ID)
	if wasLive {
		if err := s.mgr.StartLiveView(ctx); err != nil {
			return userErr(err) // connected, but the preview did not start: the error says why
		}
	}
	return nil
}

// ModeReadiness answers "can this mode start now?" without silently
// substituting another camera.
type ModeReadiness struct {
	Mode string `json:"mode"`
	// Ready: the right camera is connected and the mode can start.
	Ready    bool `json:"ready"`
	Switched bool `json:"switched"` // the preferred camera was connected just now
	// NeedsFallback: the preferred (or any suitable) camera is not the connected one; the
	// operator must approve Candidate (call PrepareMode again with its id).
	NeedsFallback bool               `json:"needsFallback"`
	Candidate     *camera.DeviceInfo `json:"candidate,omitempty"`
	Preferred     camera.Preferred   `json:"preferred"`
	Message       string             `json:"message"`
}

// PrepareMode makes the connected camera match the preference for a mode.
//   - preferred camera present → connected (safely) and Ready;
//   - preferred missing, or none chosen and the connected camera is unsuitable →
//     NeedsFallback with a Candidate; nothing is switched until the operator
//     approves by calling again with acceptID = candidate.ID.
func (s *Service) PrepareMode(mode, acceptID string) (ModeReadiness, error) {
	if mode != ModePhoto && mode != ModeVideo {
		return ModeReadiness{}, newUserErr("Unknown mode.", "")
	}
	r := ModeReadiness{Mode: mode, Preferred: s.pref(mode)}
	devs, err := s.discoverFresh()
	if err != nil && len(devs) == 0 {
		return r, userErr(err)
	}
	var usable []camera.DeviceInfo
	for _, d := range devs {
		if suitable(d, mode) {
			usable = append(usable, d)
		}
	}
	st := s.mgr.Status()
	activeID := ""
	if st.Device != nil && !st.Reconnecting && st.State != camera.StateError {
		activeID = st.Device.ID
	}
	label := map[string]string{ModePhoto: "Photo", ModeVideo: "Video"}[mode]

	// the operator approved a specific substitute
	if acceptID != "" {
		for _, d := range usable {
			if d.ID == acceptID {
				if err := s.switchTo(d.ID, false); err != nil {
					return r, err
				}
				r.Ready, r.Switched = true, d.ID != activeID
				r.Message = fmt.Sprintf("Using %s for %s mode.", d.Name, label)
				return r, nil
			}
		}
		return r, newUserErr("That camera is not suitable for "+label+" mode.", "")
	}

	if m, ok := r.Preferred.Match(usable); ok {
		if m.ID == activeID {
			r.Ready, r.Message = true, m.Name
			return r, nil
		}
		if err := s.switchTo(m.ID, false); err != nil {
			return r, err
		}
		r.Ready, r.Switched, r.Message = true, true, fmt.Sprintf("Switched to %s for %s mode.", m.Name, label)
		return r, nil
	}

	// no preference set: keep a suitable connected camera as is
	if r.Preferred.ID == "" && r.Preferred.Name == "" && activeID != "" {
		for _, d := range usable {
			if d.ID == activeID {
				r.Ready, r.Message = true, d.Name
				return r, nil
			}
		}
	}
	if len(usable) == 0 {
		r.Message = fmt.Sprintf("No camera available for %s mode.", label)
		return r, newUserErr(r.Message, map[string]string{
			ModePhoto: "Connect a camera and press Refresh on the Camera page.",
			ModeVideo: "Connect a webcam (or put a Canon body in USB streaming mode) and press Refresh.",
		}[mode])
	}
	sort.SliceStable(usable, func(i, j int) bool { return preferenceRank(usable[i], mode) < preferenceRank(usable[j], mode) })
	cand := usable[0]
	for _, d := range usable { // a suitable camera that is already connected wins
		if d.ID == activeID {
			cand = d
			break
		}
	}
	r.NeedsFallback, r.Candidate = true, &cand
	if r.Preferred.Name != "" {
		r.Message = fmt.Sprintf("Your preferred %s camera “%s” is not available.", strings.ToLower(label), r.Preferred.Name)
	} else {
		r.Message = fmt.Sprintf("No %s camera has been chosen.", strings.ToLower(label))
	}
	return r, nil
}

// RestorePreferred connects the preferred Photo camera at start-up. It never
// substitutes another camera: when the preferred one is missing the reason is
// kept in RestoreNote so the UI can ask the operator.
func (s *Service) RestorePreferred() {
	p := s.pref(ModePhoto)
	if p.ID == "" && p.Name == "" {
		return
	}
	devs, _ := s.discoverFresh()
	m, ok := p.Match(devs)
	if !ok {
		s.setRestoreNote(fmt.Sprintf("Your preferred photo camera “%s” was not found. Connect it and press Refresh, or choose another source.", p.Name))
		s.emit("camera:restore", s.restoreNote())
		return
	}
	if err := s.switchTo(m.ID, false); err != nil {
		s.setRestoreNote(fmt.Sprintf("Could not connect “%s”: %v", m.Name, err))
		s.emit("camera:restore", s.restoreNote())
		return
	}
	s.setRestoreNote("")
	s.log.Info("preferred camera restored", "device", m.Name)
}
