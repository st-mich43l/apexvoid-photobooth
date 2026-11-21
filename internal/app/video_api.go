package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/edsdk"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

// VideoState is the recording state shown in the UI.
type VideoState struct {
	State       string `json:"state"` // idle | recording | saving | error
	SessionID   string `json:"sessionId"`
	ElapsedMs   int64  `json:"elapsedMs"`
	MaxSeconds  int    `json:"maxSeconds"`
	LastMediaID string `json:"lastMediaId"`
	Error       string `json:"error,omitempty"`
	Action      string `json:"action,omitempty"`
	Notice      string `json:"notice,omitempty"`
}

type recState struct {
	st        VideoState
	mediaID   string // reserved media id
	path      string
	stopTick  chan struct{}
	start     time.Time
	finalized bool // the recording is registered exactly once (explicit stop vs. auto end race)
}

func findFFmpeg(s *Service) (string, error) { return video.Locate(s.settings.Get().FFmpegPath) }

func probeVideo(ffmpeg, path string) (video.Probe, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return video.ProbeFile(ctx, ffmpeg, path)
}

// VideoState returns the current recording state.
func (s *Service) VideoState() VideoState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.rec.st
	if st.State == "" {
		st.State = "idle"
	}
	if st.State == "recording" {
		st.ElapsedMs = time.Since(s.rec.start).Milliseconds()
	}
	return st
}

func (s *Service) setVideo(f func(*VideoState)) {
	s.mu.Lock()
	f(&s.rec.st)
	if s.rec.st.State == "" {
		s.rec.st.State = "idle"
	}
	st := s.rec.st
	s.mu.Unlock()
	s.emit("video:state", st)
}

// VideoDevices lists capture devices and their modes (cameras + microphones).
func (s *Service) VideoDevices() (video.Devices, error) {
	ff, err := findFFmpeg(s)
	if err != nil {
		return video.Devices{Video: []video.Device{}, Audio: []video.Device{}}, newUserErr("FFmpeg was not found.", "Place ffmpeg.exe next to the application or set its path in Settings. See docs/CAMERAS.md.")
	}
	ctx, cancel := s.ctx()
	defer cancel()
	d, err := video.ListDevices(ctx, ff, true)
	if d.Video == nil {
		d.Video = []video.Device{}
	}
	if d.Audio == nil {
		d.Audio = []video.Device{}
	}
	return d, userErr(err)
}

func (s *Service) pickEncoder(ff string) (string, error) {
	s.mu.Lock()
	enc := s.encoder
	s.mu.Unlock()
	if enc != "" {
		return enc, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	enc, err := video.PickEncoder(ctx, ff)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.encoder = enc
	s.mu.Unlock()
	return enc, nil
}

func (s *Service) recordable() (video.Recordable, error) {
	cam := s.mgr.Camera()
	if cam == nil {
		return nil, newUserErr("No camera is connected.", "Connect a UVC/webcam device (or a Canon body in USB streaming mode) on the Camera page.")
	}
	if rc, ok := cam.(video.Recordable); ok {
		return rc, nil
	}
	if cam.Info().Driver == edsdk.DriverName {
		return nil, newUserErr("Video recording through the Canon EDSDK is not available.",
			"Movie start/stop and movie file transfer through EDSDK are unverified on the EOS R50 and are not implemented. Set the camera's USB mode to streaming (UVC), then connect it from the UVC list to record.")
	}
	return nil, newUserErr("This camera cannot record video.", "Choose a UVC/webcam device.")
}

// StartRecording begins an MP4 recording on the connected UVC camera.
func (s *Service) StartRecording() error {
	st := s.VideoState()
	if st.State == "recording" || st.State == "saving" {
		return newUserErr("A recording is already running.", "")
	}
	if es := s.eng.Snapshot().State; es == "COUNTDOWN" || es == "CAPTURING" || es == "REVIEW" || es == "SAVING" {
		return newUserErr("A photo session is running.", "Finish it before recording video.")
	}
	rc, err := s.recordable()
	if err != nil {
		s.setVideo(func(v *VideoState) { *v = VideoState{State: "error", Error: err.Error()} })
		return err
	}
	ff, err := findFFmpeg(s)
	if err != nil {
		return newUserErr("FFmpeg was not found.", "Place ffmpeg.exe next to the application or set its path in Settings.")
	}
	enc, err := s.pickEncoder(ff)
	if err != nil {
		return newUserErr(err.Error(), "Use an FFmpeg build with libx264 or Windows Media Foundation (h264_mf). See docs/CAMERAS.md.")
	}
	ctx, cancel := s.ctx()
	defer cancel()
	if err := s.mgr.StartLiveView(ctx); err != nil {
		return userErr(err)
	}
	v := s.settings.Get().Video
	sess, err := s.store.Create("video", 1, "", s.mgr.Status().Device != nil && s.mgr.Status().Device.Simulated)
	if err != nil {
		return userErr(err)
	}
	id, path, err := s.store.NewVideoPath(sess.ID)
	if err != nil {
		return userErr(err)
	}
	rc.SetOnRecordEnd(func(r video.RecordResult) { s.finishRecording(r) })
	if err := rc.StartRecording(ctx, video.RecordOptions{Path: path, Encoder: enc, Audio: v.AudioInput, MaxDuration: time.Duration(v.MaxSeconds) * time.Second}); err != nil {
		_ = s.store.SetStatus(sess.ID, sessions.StatusFailed, err.Error())
		return userErr(err)
	}
	s.mu.Lock()
	s.rec.mediaID, s.rec.path, s.rec.start = id, path, time.Now()
	s.rec.finalized = false
	s.rec.stopTick = make(chan struct{})
	tick := s.rec.stopTick
	s.mu.Unlock()
	notice := ""
	if v.AudioInput == "" {
		notice = "Recording without audio (no microphone selected)."
	}
	s.setVideo(func(x *VideoState) {
		*x = VideoState{State: "recording", SessionID: sess.ID, MaxSeconds: v.MaxSeconds, Notice: notice}
	})
	s.log.Info("recording started", "session", sess.ID, "encoder", enc, "audio", v.AudioInput)
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-tick:
				return
			case <-t.C:
				s.emit("video:state", s.VideoState())
			}
		}
	}()
	return nil
}

// StopRecording ends the recording and stores the MP4 in the library.
func (s *Service) StopRecording() error {
	if s.VideoState().State != "recording" {
		return newUserErr("No recording is running.", "")
	}
	rc, err := s.recordable()
	if err != nil {
		return err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	s.setVideo(func(v *VideoState) { v.State = "saving" })
	res, err := rc.StopRecording(ctx)
	if err != nil {
		s.setVideo(func(v *VideoState) { v.State, v.Error = "error", err.Error() })
		return userErr(err)
	}
	s.finishRecording(res)
	return nil
}

// finishRecording registers the MP4 (once), whether it ended by Stop, by the
// duration limit, or because the device vanished (partial but playable).
func (s *Service) finishRecording(res video.RecordResult) {
	s.mu.Lock()
	first := !s.rec.finalized
	s.rec.finalized = true
	id, path, tick := s.rec.mediaID, s.rec.path, s.rec.stopTick
	sessID := s.rec.st.SessionID
	s.rec.stopTick = nil
	s.mu.Unlock()
	if !first || id == "" {
		return
	}
	if tick != nil {
		close(tick)
	}
	s.setVideo(func(v *VideoState) { v.State = "saving" })
	in := sessions.VideoInput{AbsPath: path, Partial: res.Partial, Source: "UVC stream recorded by FFmpeg"}
	var verr error
	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		verr = errors.New("the recording file is empty")
	} else if ff, err := findFFmpeg(s); err == nil {
		if p, err := probeVideo(ff, path); err == nil {
			in.DurationMs, in.Width, in.Height = p.Duration.Milliseconds(), p.Width, p.Height
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			at := p.Duration / 2
			if at > time.Second {
				at = time.Second
			}
			if th, err := video.Thumbnail(ctx, ff, path, at, 420); err == nil {
				in.ThumbJPEG = th
			}
			cancel()
		} else if res.Partial {
			verr = fmt.Errorf("the partial recording could not be read: %w", err)
		}
	}
	if verr != nil {
		_ = s.store.SetStatus(sessID, sessions.StatusFailed, verr.Error())
		s.setVideo(func(v *VideoState) { *v = VideoState{State: "error", Error: verr.Error()} })
		return
	}
	m, err := s.store.AddVideo(sessID, id, in)
	if err != nil {
		s.setVideo(func(v *VideoState) { *v = VideoState{State: "error", Error: err.Error()} })
		return
	}
	status := sessions.StatusCompleted
	if res.Partial {
		status = sessions.StatusInterrupted
	}
	_ = s.store.SetStatus(sessID, status, res.Reason)
	notice := ""
	switch {
	case res.Partial:
		notice = "Recording ended unexpectedly (" + res.Reason + "); the part that was saved is in the gallery."
	case res.Reason == "max-duration":
		notice = "Maximum duration reached; recording saved."
	}
	s.log.Info("recording saved", "media", m.ID, "ms", in.DurationMs, "partial", res.Partial, "reason", res.Reason)
	s.setVideo(func(v *VideoState) {
		*v = VideoState{State: "idle", LastMediaID: m.ID, Notice: notice, SessionID: sessID}
	})
}

func (s *Service) stopRecordingForShutdown() {
	if s.VideoState().State != "recording" {
		return
	}
	if rc, err := s.recordable(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if res, err := rc.StopRecording(ctx); err == nil {
			s.finishRecording(res)
		}
	}
}
