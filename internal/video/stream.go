package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// RecordResult describes a finished recording.
type RecordResult struct {
	Path     string
	Duration time.Duration // wall-clock; use ProbeFile for the file's own duration
	Partial  bool          // stopped abnormally; the fragmented MP4 is still playable up to that point
	Reason   string        // "stopped", "max-duration", "device-lost", "killed"
	Err      error
}

// StreamOptions configures a Stream.
type StreamOptions struct {
	FFmpeg string
	Source Source
	// OnFrame receives every preview JPEG.
	OnFrame func([]byte)
	// OnLost is called when ffmpeg dies while nobody asked it to stop
	// (typically the USB camera was unplugged).
	OnLost func(err error, detail string)
	// OnRecordEnd is called when a recording ends by itself (max duration
	// reached or the device vanished). Explicit StopRecording does not call it.
	OnRecordEnd func(RecordResult)
}

// Stream owns the capture device through a single ffmpeg process that gives a
// JPEG preview and, on demand, an MP4 recording. Starting or stopping a
// recording restarts the process (the preview freezes for ~1 s) because a
// capture device cannot be opened by two processes.
type Stream struct {
	o  StreamOptions
	mu sync.Mutex

	proc      *Process
	recording *RecordOptions
	recStart  time.Time
	stopping  bool
	timer     *time.Timer
}

// NewStream creates an idle stream.
func NewStream(o StreamOptions) *Stream { return &Stream{o: o} }

// Start opens the device and begins the preview.
func (s *Stream) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc != nil {
		return nil
	}
	return s.startLocked(ctx, nil)
}

func (s *Stream) startLocked(ctx context.Context, rec *RecordOptions) error {
	src := s.o.Source
	src.Audio = "" // the microphone is only opened while recording
	if rec != nil {
		src.Audio = rec.Audio
	}
	p, err := StartPipeline(ctx, PipelineOptions{FFmpeg: s.o.FFmpeg, Source: src, Record: rec}, s.o.OnFrame)
	if err != nil {
		return err
	}
	s.proc, s.recording = p, rec
	if rec != nil {
		s.recStart = time.Now()
		if rec.MaxDuration > 0 {
			s.timer = time.AfterFunc(rec.MaxDuration, s.autoStop)
		}
	}
	go s.watch(p)
	return nil
}

// watch reacts to an ffmpeg process ending that was not stopped by us.
func (s *Stream) watch(p *Process) {
	<-p.Done()
	s.mu.Lock()
	if s.proc != p || s.stopping {
		s.mu.Unlock()
		return
	}
	rec, start := s.recording, s.recStart
	s.proc, s.recording = nil, nil
	s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	if rec != nil {
		res := RecordResult{Path: rec.Path, Duration: time.Since(start), Reason: "device-lost", Partial: true, Err: fmt.Errorf("%s", Explain(p.Stderr()))}
		if s.o.OnRecordEnd != nil {
			s.o.OnRecordEnd(res)
		}
	}
	if s.o.OnLost != nil {
		s.o.OnLost(p.Err(), Explain(p.Stderr()))
	}
}

// Recording reports whether a recording is active and when it started.
func (s *Stream) Recording() (bool, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recording != nil, s.recStart
}

// StartRecording restarts the pipeline with an MP4 output.
func (s *Stream) StartRecording(ctx context.Context, rec RecordOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc == nil {
		return errors.New("live view must be running before recording")
	}
	if s.recording != nil {
		return errors.New("already recording")
	}
	if rec.Encoder == "" {
		return errors.New("no H.264 encoder selected")
	}
	old := s.proc
	s.stopping = true
	s.mu.Unlock()
	_ = old.Stop(5 * time.Second)
	s.mu.Lock()
	s.stopping = false
	s.proc = nil
	if err := s.startLocked(ctx, &rec); err != nil {
		_ = s.startLocked(ctx, nil) // best effort: restore the preview
		return err
	}
	return nil
}

// StopRecording finalises the MP4 and brings the preview back.
func (s *Stream) StopRecording(ctx context.Context) (RecordResult, error) {
	return s.finish(ctx, "stopped")
}

// autoStop runs when the maximum duration elapses.
func (s *Stream) autoStop() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := s.finish(ctx, "max-duration")
	if err == nil && s.o.OnRecordEnd != nil {
		s.o.OnRecordEnd(res)
	}
}

func (s *Stream) finish(ctx context.Context, reason string) (RecordResult, error) {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.recording == nil {
		s.mu.Unlock()
		return RecordResult{}, errors.New("not recording")
	}
	rec, start, p := *s.recording, s.recStart, s.proc
	s.stopping = true
	s.mu.Unlock()
	serr := p.Stop(10 * time.Second)
	res := RecordResult{Path: rec.Path, Duration: time.Since(start), Reason: reason}
	if serr != nil {
		res.Partial, res.Reason, res.Err = true, "killed", serr
	}
	if st, err := os.Stat(rec.Path); err != nil || st.Size() == 0 {
		res.Partial, res.Err = true, errors.New("recording file is empty")
	}
	s.mu.Lock()
	s.stopping = false
	s.proc, s.recording = nil, nil
	err := s.startLocked(ctx, nil)
	s.mu.Unlock()
	if err != nil {
		res.Err = errors.Join(res.Err, fmt.Errorf("preview did not restart: %w", err))
	}
	return res, nil
}

// Stop shuts everything down; an active recording is finalised first.
func (s *Stream) Stop() {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	p := s.proc
	s.stopping = true
	s.mu.Unlock()
	if p != nil {
		_ = p.Stop(8 * time.Second)
	}
	s.mu.Lock()
	s.proc, s.recording, s.stopping = nil, nil, false
	s.mu.Unlock()
}

// Recordable is implemented by cameras that can record video streamed to the
// PC (UVC). Canon EDSDK movie recording is a separate, unverified path and is
// intentionally not exposed through this interface.
type Recordable interface {
	StartRecording(ctx context.Context, rec RecordOptions) error
	StopRecording(ctx context.Context) (RecordResult, error)
	Recording() (active bool, since time.Time)
	// SetOnRecordEnd registers a callback for recordings that end by themselves.
	SetOnRecordEnd(func(RecordResult))
}
