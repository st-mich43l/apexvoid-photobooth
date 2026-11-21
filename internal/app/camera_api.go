package app

import (
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

// BootInfo is everything the UI needs on first paint.
type BootInfo struct {
	Version    string            `json:"version"`
	DataDir    string            `json:"dataDir"`
	MediaBase  string            `json:"mediaBase"` // http://127.0.0.1:port (loopback only)
	MediaToken string            `json:"mediaToken"`
	Settings   settings.Settings `json:"settings"`
	Camera     camera.Status     `json:"camera"`
	Capture    capture.Snapshot  `json:"capture"`
	Video      VideoState        `json:"video"`
	Library    LibrarySummary    `json:"library"`
	Location   LibraryStatus     `json:"location"`
	OS         string            `json:"os"`
}

// Bootstrap returns the initial application state.
func (s *Service) Bootstrap() BootInfo {
	return BootInfo{
		Version: s.opts.Version, DataDir: s.db.Layout.Root, MediaBase: s.httpd.base, MediaToken: s.httpd.token,
		Settings: s.publicSettings(), Camera: s.mgr.Status(), Capture: s.eng.Snapshot(), Video: s.VideoState(),
		Library: s.librarySummary(), Location: s.LibraryStatus(), OS: osName(),
	}
}

// ListCameras discovers connected cameras from every available driver.
func (s *Service) ListCameras() ([]camera.DeviceInfo, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	devs, err := s.mgr.Discover(ctx)
	if devs == nil {
		devs = []camera.DeviceInfo{}
	}
	return devs, userErr(err)
}

// CameraStatus returns the camera subsystem state.
func (s *Service) CameraStatus() camera.Status { return s.mgr.Status() }

// ConnectCamera connects the discovered source with the given id, using the
// safe switching sequence (see switchTo).
func (s *Service) ConnectCamera(id string) error { return s.switchTo(id, false) }

// DisconnectCamera releases the camera.
func (s *Service) DisconnectCamera() error {
	_ = s.eng.Reset()
	s.stopRecordingForShutdown()
	return userErr(s.mgr.Disconnect())
}

// StartLiveView starts the preview outside of a shooting session (Studio camera page).
func (s *Service) StartLiveView() error {
	ctx, cancel := s.ctx()
	defer cancel()
	return userErr(s.mgr.StartLiveView(ctx))
}

// StopLiveView stops the preview.
func (s *Service) StopLiveView() error { return userErr(s.mgr.StopLiveView()) }
