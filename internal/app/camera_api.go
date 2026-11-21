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
	OS         string            `json:"os"`
}

// Bootstrap returns the initial application state.
func (s *Service) Bootstrap() BootInfo {
	return BootInfo{
		Version: s.opts.Version, DataDir: s.db.Layout.Root, MediaBase: s.httpd.base, MediaToken: s.httpd.token,
		Settings: s.settings.Get(), Camera: s.mgr.Status(), Capture: s.eng.Snapshot(), Video: s.VideoState(),
		Library: s.librarySummary(), OS: osName(),
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

// ConnectCamera connects the discovered device with the given id.
func (s *Service) ConnectCamera(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	// the shooting engine must be idle: a connect swaps the camera under it
	if st := s.eng.Snapshot().State; st != capture.Idle && st != capture.Preview && st != capture.Completed && st != capture.Cancelled && st != capture.Error {
		return newUserErr("A photo session is running.", "Finish or cancel it before changing the camera.")
	}
	_ = s.eng.Reset()
	devs, err := s.mgr.Discover(ctx)
	if err != nil {
		return userErr(err)
	}
	for _, d := range devs {
		if d.ID == id {
			if err := s.mgr.Connect(ctx, d); err != nil {
				s.log.Warn("connect failed", "device", d.Name, "err", err)
				return userErr(err)
			}
			s.log.Info("camera connected", "device", d.Name, "driver", d.Driver)
			return nil
		}
	}
	return newUserErr("That camera is no longer available.", "Press Refresh and choose it again.")
}

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
