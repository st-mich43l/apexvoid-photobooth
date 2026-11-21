package video

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Source is a capture device selection plus the mode to open it in.
type Source struct {
	Video  Device
	Audio  string // audio device name/id ("" = record without audio)
	Width  int
	Height int
	FPS    int
	Format string // "" = driver default; "mjpeg" for camera-compressed

	// Test hooks: when InputOverride is set it replaces the platform input
	// arguments (used to feed FFmpeg a synthetic lavfi source in CI).
	InputOverride []string
	AudioMap      string

	// Fallback levels (see Attempts): drop the strict DirectShow buffering
	// options, or open the device by its friendly name instead of its path.
	Minimal   bool
	ForceName bool
}

// Describe is a short human description of how the device is opened.
func (s Source) Describe() string {
	mode := "device default mode"
	if s.Width > 0 {
		mode = fmt.Sprintf("%dx%d", s.Width, s.Height)
		if s.FPS > 0 {
			mode += fmt.Sprintf("@%d", s.FPS)
		}
		if s.Format != "" {
			mode += " " + s.Format
		}
	}
	id := "by id"
	if s.ForceName || !strings.HasPrefix(s.Video.ID, "@device") {
		id = "by name"
	}
	if s.Minimal {
		id += ", minimal options"
	}
	return mode + ", " + id
}

// Attempts lists the ways a device is tried, strictest first. Drivers differ
// (infrared/Windows Hello streams, cameras that list modes they then refuse,
// devices that reject buffering options), so opening walks down this ladder
// until a frame arrives instead of failing on the first "Invalid argument".
func (s Source) Attempts() []Source {
	if s.InputOverride != nil {
		return []Source{s}
	}
	var out []Source
	seen := map[string]bool{}
	add := func(x Source) {
		args, am := x.inputArgs()
		k := strings.Join(args, "\x00") + am
		if !seen[k] {
			seen[k] = true
			out = append(out, x)
		}
	}
	add(s)
	noFmt := s
	noFmt.Format = ""
	add(noFmt)
	dflt := s
	dflt.Format, dflt.Width, dflt.Height, dflt.FPS = "", 0, 0, 0
	add(dflt)
	named := dflt
	named.ForceName = true
	add(named)
	minimal := dflt
	minimal.Minimal = true
	add(minimal)
	minNamed := minimal
	minNamed.ForceName = true
	add(minNamed)
	return out
}

// RecordOptions configures the MP4 output of a pipeline.
type RecordOptions struct {
	Path        string
	Encoder     string
	Audio       string // microphone device ("" = no audio track)
	MaxDuration time.Duration
}

// PipelineOptions describes one ffmpeg process: JPEG preview on stdout and,
// optionally, an MP4 file. One process owns the device because DirectShow and
// V4L2 devices cannot be opened twice.
type PipelineOptions struct {
	FFmpeg         string
	Source         Source
	PreviewFPS     int // default 15
	PreviewQuality int // mjpeg -q:v, default 3 (lower = better)
	Record         *RecordOptions
	// FirstFrameTimeout bounds the wait for the first preview frame (default 20 s).
	FirstFrameTimeout time.Duration
}

func (s Source) inputArgs() (args []string, audioMap string) {
	if s.InputOverride != nil {
		return s.InputOverride, s.AudioMap
	}
	size := fmt.Sprintf("%dx%d", s.Width, s.Height)
	fps := strconv.Itoa(s.FPS)
	switch runtime.GOOS {
	case "windows":
		args = []string{"-f", "dshow"}
		if !s.Minimal {
			args = append(args, "-rtbufsize", "256M", "-thread_queue_size", "1024", "-use_wallclock_as_timestamps", "1")
		}
		if s.Width > 0 {
			args = append(args, "-video_size", size)
		}
		if s.FPS > 0 {
			args = append(args, "-framerate", fps)
		}
		switch s.Format {
		case "":
		case "mjpeg", "h264", "hevc":
			args = append(args, "-vcodec", s.Format)
		default:
			args = append(args, "-pixel_format", s.Format)
		}
		// the alternative name (@device_pnp_...) is unambiguous when two cameras share a name
		devName := s.Video.Name
		if strings.HasPrefix(s.Video.ID, "@device") && !s.ForceName {
			devName = s.Video.ID
		}
		in := "video=" + devName
		if s.Audio != "" {
			in += ":audio=" + s.Audio
			audioMap = "0:a"
		}
		return append(args, "-i", in), audioMap
	case "darwin":
		args = []string{"-f", "avfoundation"}
		if s.Width > 0 {
			args = append(args, "-video_size", size)
		}
		if s.FPS > 0 {
			args = append(args, "-framerate", fps)
		}
		in := s.Video.ID + ":none"
		if s.Audio != "" {
			in = s.Video.ID + ":" + s.Audio
			audioMap = "0:a"
		}
		return append(args, "-i", in), audioMap
	default:
		args = []string{"-f", "v4l2", "-thread_queue_size", "1024"}
		if s.Width > 0 {
			args = append(args, "-video_size", size)
		}
		if s.FPS > 0 {
			args = append(args, "-framerate", fps)
		}
		switch s.Format {
		case "":
		case "mjpeg":
			args = append(args, "-input_format", "mjpeg")
		default:
			args = append(args, "-input_format", s.Format)
		}
		args = append(args, "-i", s.Video.ID)
		if s.Audio != "" {
			args = append(args, "-f", "pulse", "-thread_queue_size", "1024", "-i", s.Audio)
			audioMap = "1:a"
		}
		return args, audioMap
	}
}

func encoderArgs(enc string, fps int) []string {
	// 1 s GOP: fragments are cut at keyframes, so a crash loses at most ~1-2 s of video
	g := strconv.Itoa(max(fps, 1))
	switch enc {
	case "libx264":
		return []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-g", g}
	case "h264_nvenc":
		return []string{"-c:v", enc, "-preset", "p4", "-b:v", "8M", "-g", g}
	default: // h264_mf, h264_qsv, h264_amf, libopenh264
		return []string{"-c:v", enc, "-b:v", "8M", "-g", g}
	}
}

// BuildArgs returns the ffmpeg command line (without the executable).
func BuildArgs(o PipelineOptions) []string {
	pfps := o.PreviewFPS
	if pfps <= 0 {
		pfps = 15
	}
	q := o.PreviewQuality
	if q <= 0 {
		q = 3
	}
	in, audioMap := o.Source.inputArgs()
	// no -nostdin: ffmpeg reads "q" on stdin for a graceful stop that finalises the MP4
	a := []string{"-hide_banner", "-nostats", "-loglevel", "warning"}
	a = append(a, in...)
	// Output 1: JPEG preview frames on stdout (also the source of stills).
	a = append(a, "-map", "0:v", "-an", "-vf", fmt.Sprintf("fps=%d", pfps), "-c:v", "mjpeg", "-q:v", strconv.Itoa(q), "-f", "image2pipe", "pipe:1")
	if r := o.Record; r != nil {
		fps := o.Source.FPS
		if fps <= 0 {
			fps = 30
		}
		a = append(a, "-map", "0:v")
		if audioMap != "" {
			a = append(a, "-map", audioMap)
		}
		a = append(a, encoderArgs(r.Encoder, fps)...)
		a = append(a, "-pix_fmt", "yuv420p", "-r", strconv.Itoa(fps), "-fps_mode", "cfr")
		if audioMap != "" {
			// async=1000 stretches/squeezes audio to the video clock: A/V sync.
			a = append(a, "-c:a", "aac", "-b:a", "160k", "-ar", "48000", "-af", "aresample=async=1000:first_pts=0")
		}
		// Fragmented MP4: the file stays playable if the app or PC dies mid-recording.
		a = append(a, "-flush_packets", "1", "-movflags", "+frag_keyframe+empty_moov+default_base_moof")
		a = append(a, "-f", "mp4", "-y", r.Path)
	}
	return a
}

// Process is a running ffmpeg pipeline.
type Process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	done   chan struct{}
	mu     sync.Mutex
	err    error
	stderr *tailBuffer
	frames int
}

// OpenError is returned when ffmpeg could not deliver a first frame. It keeps
// the complete command line and log so the real cause can be diagnosed.
type OpenError struct {
	Args   []string
	Stderr string
	Reason string
}

func (e *OpenError) Error() string { return e.Reason }

// StartPipeline launches ffmpeg and waits for the first preview frame. onFrame
// is called for every JPEG frame from a reader goroutine.
func StartPipeline(ctx context.Context, o PipelineOptions, onFrame func([]byte)) (*Process, error) {
	cmd := exec.Command(o.FFmpeg, BuildArgs(o)...)
	hide(cmd)
	ffTimeout := o.FirstFrameTimeout
	if ffTimeout <= 0 {
		ffTimeout = 20 * time.Second
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &Process{cmd: cmd, stdin: stdin, done: make(chan struct{}), stderr: &tailBuffer{max: 8192}}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	first := make(chan struct{})
	var once sync.Once
	go func() {
		readFrames(stdout, func(f []byte) {
			p.mu.Lock()
			p.frames++
			p.mu.Unlock()
			once.Do(func() { close(first) })
			onFrame(f)
		})
		werr := cmd.Wait()
		p.mu.Lock()
		p.err = werr
		p.mu.Unlock()
		close(p.done)
	}()
	select {
	case <-first:
		return p, nil
	case <-p.done:
		return nil, &OpenError{Args: cmd.Args, Stderr: p.stderr.String(), Reason: fmt.Sprintf("ffmpeg exited before the first frame: %s", Explain(p.stderr.String()))}
	case <-time.After(ffTimeout):
		p.Kill()
		return nil, &OpenError{Args: cmd.Args, Stderr: p.stderr.String(), Reason: fmt.Sprintf("camera produced no frames within %d s: %s", int(ffTimeout.Seconds()), Explain(p.stderr.String()))}
	case <-ctx.Done():
		p.Kill()
		return nil, ctx.Err()
	}
}

// Done is closed when ffmpeg has exited.
func (p *Process) Done() <-chan struct{} { return p.done }

// Err returns ffmpeg's exit error (nil on clean exit); valid after Done.
func (p *Process) Err() error { p.mu.Lock(); defer p.mu.Unlock(); return p.err }

// Stderr returns the tail of ffmpeg's log output.
func (p *Process) Stderr() string { return p.stderr.String() }

// Stop asks ffmpeg to finish cleanly (so the MP4 is finalised), then kills
// it after grace.
func (p *Process) Stop(grace time.Duration) error {
	_, _ = io.WriteString(p.stdin, "q")
	_ = p.stdin.Close()
	select {
	case <-p.done:
		return nil
	case <-time.After(grace):
		p.Kill()
		<-p.done
		return errors.New("ffmpeg did not stop in time and was killed; the recording is partial")
	}
}

// Kill terminates ffmpeg immediately.
func (p *Process) Kill() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

// readFrames splits an MJPEG byte stream into JPEG images (SOI..EOI). Inside
// entropy-coded data 0xFF is always stuffed with 0x00, so FFD9 only ever marks EOI.
func readFrames(r io.Reader, emit func([]byte)) {
	br := bufio.NewReaderSize(r, 1<<20)
	var frame []byte
	in := false
	var prev byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		if !in {
			if prev == 0xFF && b == 0xD8 {
				in = true
				frame = append(frame[:0], 0xFF, 0xD8)
			}
			prev = b
			continue
		}
		frame = append(frame, b)
		if prev == 0xFF && b == 0xD9 {
			out := make([]byte, len(frame))
			copy(out, frame)
			emit(out)
			in = false
			prev = 0
			continue
		}
		prev = b
	}
}

type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(bytes.ToValidUTF8(t.buf, nil)))
}

// Explain turns ffmpeg's error text into an operator-facing hint. The common
// real-world failure is another program owning the camera.
func Explain(stderr string) string {
	l := strings.ToLower(stderr)
	last := stderr
	if i := strings.LastIndex(strings.TrimSpace(stderr), "\n"); i >= 0 {
		last = strings.TrimSpace(stderr)[i+1:]
	}
	switch {
	case strings.Contains(l, "could not run graph"), strings.Contains(l, "could not set video options"),
		strings.Contains(l, "device or resource busy"), strings.Contains(l, "i/o error"), strings.Contains(l, "already in use"):
		return "the camera could not be opened. It is probably in use by another program (EOS Utility, OBS, Zoom, a browser) — close it and retry. " +
			"For a Canon EOS body, set its USB mode to streaming/webcam. Details: " + last
	case strings.Contains(l, "could not find video device"), strings.Contains(l, "no such file or directory"):
		return "the camera was not found. Reconnect it and press Refresh. Details: " + last
	case strings.Contains(l, "permission denied"), strings.Contains(l, "access is denied"):
		return "access to the camera was denied. Check the OS camera/microphone privacy settings. Details: " + last
	}
	if last == "" {
		return "no details from ffmpeg"
	}
	return last
}
