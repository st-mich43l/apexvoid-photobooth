package video

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const dshowModern = `[dshow @ 0000020d0f4cc0c0] "Integrated Webcam" (video)
[dshow @ 0000020d0f4cc0c0]   Alternative name "@device_pnp_\\?\usb#vid_0bda&pid_5830&mi_00#7&1f#{65e8773d}\global"
[dshow @ 0000020d0f4cc0c0] "Canon EOS R50" (video)
[dshow @ 0000020d0f4cc0c0]   Alternative name "@device_pnp_\\?\usb#vid_04a9&pid_32ea#6&2#{65e8773d}\global"
[dshow @ 0000020d0f4cc0c0] "Microphone Array (Realtek(R) Audio)" (audio)
[dshow @ 0000020d0f4cc0c0]   Alternative name "@device_cw_{33D9A762-90C8-11D0-BD43-00A0C911CE86}\wave_{AB}"
[dshow @ 0000020d0f4cc0c0] "OBS Virtual Camera" (none)
dummy: Immediate exit requested`

const dshowOld = `[dshow @ 03ac3dc0] DirectShow video devices (some may be both video and audio devices)
[dshow @ 03ac3dc0]  "USB2.0 HD UVC WebCam"
[dshow @ 03ac3dc0]     Alternative name "@device_pnp_x"
[dshow @ 03ac3dc0] DirectShow audio devices
[dshow @ 03ac3dc0]  "Microphone (USB2.0 MIC)"
[dshow @ 03ac3dc0]     Alternative name "@device_cw_y"`

func TestParseDShowDevices(t *testing.T) {
	d := parseDShowDevices(dshowModern)
	if len(d.Video) != 2 || len(d.Audio) != 1 {
		t.Fatalf("%+v", d)
	}
	if d.Video[1].Name != "Canon EOS R50" || !strings.HasPrefix(d.Video[1].ID, "@device_pnp_") {
		t.Fatalf("%+v", d.Video[1])
	}
	d = parseDShowDevices(dshowOld)
	if len(d.Video) != 1 || d.Video[0].Name != "USB2.0 HD UVC WebCam" || len(d.Audio) != 1 || d.Audio[0].Name != "Microphone (USB2.0 MIC)" {
		t.Fatalf("old format: %+v", d)
	}
}

func TestDeviceKeys(t *testing.T) {
	usb, key := DeviceKeys(`@device_pnp_\\?\usb#vid_04A9&pid_32EA#6&2a&0&2#{65e8773d-8f56-11d0-a3b9-00a0c9223196}\global`)
	if usb != "04a9:32ea" || key != "usb#vid_04a9&pid_32ea#6&2a&0&2" {
		t.Fatalf("%q %q", usb, key)
	}
	// composite devices carry the interface: the key keeps it so RGB (mi_00) and IR (mi_02) differ
	usb2, key2 := DeviceKeys(`@device_pnp_\\?\usb#vid_13d3&pid_56cb&mi_00#6&2a&0&0000#{x}\global`)
	usb3, key3 := DeviceKeys(`@device_pnp_\\?\usb#vid_13d3&pid_56cb&mi_02#6&2a&0&0002#{x}\global`)
	if usb2 != "13d3:56cb" || usb2 != usb3 || key2 == key3 || !strings.Contains(key2, "&mi_00#") {
		t.Fatalf("%q %q | %q %q", usb2, key2, usb3, key3)
	}
	if u, k := DeviceKeys(`@device_sw_{860BB310-5D01-11D0-BD3B-00A0C911CE86}\{A3FCE0F5}`); u != "" || k != "" {
		t.Fatal("software filters have no USB key")
	}
	d := parseDShowDevices(dshowModern)
	if d.Video[1].USBID != "04a9:32ea" || d.Video[1].Key == "" {
		t.Fatalf("%+v", d.Video[1])
	}
}

func TestAlternativeNameIsUsedToOpenDuplicateNamedCameras(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DirectShow arguments are Windows-only")
	}
	o := PipelineOptions{FFmpeg: "ffmpeg", Source: Source{Video: Device{Name: "USB Camera", ID: `@device_pnp_\\?\usb#vid_1&pid_2#a#{g}\global`}, Width: 640, Height: 480, FPS: 30}}
	if j := strings.Join(BuildArgs(o), " "); !strings.Contains(j, `video=@device_pnp_`) {
		t.Fatalf("%s", j)
	}
}

func TestParseDShowModes(t *testing.T) {
	out := `[dshow @ 0x1] DirectShow video device options (from video devices)
[dshow @ 0x1]  Pin "Capture" (alternative pin name "0")
[dshow @ 0x1]   pixel_format=yuyv422  min s=640x480 fps=5 max s=640x480 fps=30
[dshow @ 0x1]   pixel_format=yuyv422  min s=1280x720 fps=5 max s=1280x720 fps=10
[dshow @ 0x1]   vcodec=mjpeg  min s=1920x1080 fps=5 max s=1920x1080 fps=30
[dshow @ 0x1]   vcodec=mjpeg  min s=1280x720 fps=5 max s=1280x720 fps=30
[dshow @ 0x1]   vcodec=mjpeg  min s=1280x720 fps=5 max s=1280x720 fps=30 (tv, bt709)`
	m := parseDShowModes(out)
	if len(m) != 4 {
		t.Fatalf("%+v", m)
	}
	if m[0].Width != 1920 || m[0].Format != "mjpeg" || m[0].FPSMax != 30 {
		t.Fatalf("not sorted best-first: %+v", m)
	}
}

func TestParseV4LAndAVF(t *testing.T) {
	v := `[video4linux2,v4l2 @ 0x5] Raw       :     yuyv422 :           YUYV 4:2:2 : 640x480 320x240
[video4linux2,v4l2 @ 0x5] Compressed:       mjpeg :          Motion-JPEG : 1280x720 640x480`
	m := parseV4LModes(v)
	if len(m) != 4 || m[0].Width != 1280 || m[0].Format != "mjpeg" {
		t.Fatalf("%+v", m)
	}
	a := parseAVFDevices(`[AVFoundation indev @ 0x1] AVFoundation video devices:
[AVFoundation indev @ 0x1] [0] FaceTime HD Camera
[AVFoundation indev @ 0x1] AVFoundation audio devices:
[AVFoundation indev @ 0x1] [0] MacBook Microphone`)
	if len(a.Video) != 1 || len(a.Audio) != 1 || a.Video[0].Name != "FaceTime HD Camera" {
		t.Fatalf("%+v", a)
	}
}

func TestParseProbe(t *testing.T) {
	p := parseProbe(`  Duration: 00:00:05.04, start: 0.000000, bitrate: 1200 kb/s
  Stream #0:0: Video: h264 (High) (avc1 / 0x31637661), yuv420p(tv, progressive), 1280x720 [SAR 1:1 DAR 16:9], 30 fps
  Stream #0:1: Audio: aac (LC) (mp4a / 0x6D703461), 48000 Hz, stereo, fltp, 160 kb/s`)
	if !p.HasVideo || !p.HasAudio || p.Width != 1280 || p.Height != 720 || p.Duration != 5040*time.Millisecond || p.VCodec != "h264" {
		t.Fatalf("%+v", p)
	}
}

func TestPickEncoder(t *testing.T) {
	list := ` V....D libx264              libx264 H.264
 V..... h264_mf              H.264 via MediaFoundation
 A....D aac                  AAC`
	if e, err := pickEncoder(list); err != nil || e != "libx264" {
		t.Fatalf("%s %v", e, err)
	}
	if e, _ := pickEncoder(" V..... h264_mf  MF\n V....D h264_nvenc NV"); e != "h264_mf" {
		t.Fatalf("LGPL build should pick h264_mf, got %s", e)
	}
	if _, err := pickEncoder(" V....D mpeg4  MPEG-4"); err == nil {
		t.Fatal("no H.264 encoder must be an error")
	}
}

func TestBuildArgsRecordingAndPreview(t *testing.T) {
	o := PipelineOptions{FFmpeg: "ffmpeg", Source: Source{Video: Device{Name: "Cam"}, Audio: "Mic", Width: 1280, Height: 720, FPS: 30, Format: "mjpeg"},
		Record: &RecordOptions{Path: "out.mp4", Encoder: "libx264", MaxDuration: 30 * time.Second}}
	j := strings.Join(BuildArgs(o), " ")
	for _, want := range []string{"-f image2pipe pipe:1", "-c:v libx264", "-c:a aac", "frag_keyframe+empty_moov", "-fps_mode cfr", "aresample=async=1000", "out.mp4"} {
		if !strings.Contains(j, want) {
			t.Errorf("args missing %q:\n%s", want, j)
		}
	}
	o.Record = nil
	if j := strings.Join(BuildArgs(o), " "); strings.Contains(j, "libx264") || !strings.Contains(j, "pipe:1") {
		t.Errorf("preview-only args wrong: %s", j)
	}
	o.Source.Audio = ""
	o.Record = &RecordOptions{Path: "o.mp4", Encoder: "h264_mf"}
	if j := strings.Join(BuildArgs(o), " "); strings.Contains(j, "-c:a aac") {
		t.Errorf("no microphone selected must give a silent-free file without audio track: %s", j)
	}
}

func TestExplainBusyCamera(t *testing.T) {
	e := Explain("[dshow @ 000] Could not run graph (sorry!)\nError opening input: I/O error")
	if !strings.Contains(e, "another program") || !strings.Contains(e, "streaming") {
		t.Fatal(e)
	}
}

func TestReadFramesSplitsStream(t *testing.T) {
	mk := func(c uint8) []byte {
		img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
		for i := range img.Pix {
			img.Pix[i] = c
		}
		var b bytes.Buffer
		_ = jpeg.Encode(&b, img, nil)
		return b.Bytes()
	}
	var stream bytes.Buffer
	for _, c := range []uint8{10, 128, 250} {
		stream.Write(mk(c))
	}
	var got [][]byte
	readFrames(&stream, func(f []byte) { got = append(got, f) })
	if len(got) != 3 {
		t.Fatalf("frames=%d", len(got))
	}
	for _, f := range got {
		if _, err := jpeg.Decode(bytes.NewReader(f)); err != nil {
			t.Fatal(err)
		}
	}
	_ = color.Black
}

// --- integration with a synthetic lavfi source (needs ffmpeg on PATH) ---

func lavfiSource() Source {
	return Source{Width: 640, Height: 360, FPS: 30, AudioMap: "1:a",
		InputOverride: []string{"-re", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30", "-re", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000"}}
}

func needFFmpeg(t *testing.T) (string, string) {
	t.Helper()
	ff, err := Locate("")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	enc, err := PickEncoder(context.Background(), ff)
	if err != nil {
		t.Skip(err)
	}
	return ff, enc
}

func TestStreamPreviewAndRecordingWithSyntheticSource(t *testing.T) {
	ff, enc := needFFmpeg(t)
	frames := make(chan []byte, 256)
	var ended []RecordResult
	s := NewStream(StreamOptions{FFmpeg: ff, Source: lavfiSource(),
		OnFrame: func(b []byte) {
			select {
			case frames <- b:
			default:
			}
		},
		OnRecordEnd: func(r RecordResult) { ended = append(ended, r) }})
	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	f := <-frames
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(f))
	if err != nil || cfg.Width != 640 {
		t.Fatalf("preview frame: %v %+v", err, cfg)
	}

	out := filepath.Join(t.TempDir(), "rec.mp4")
	if err := s.StartRecording(ctx, RecordOptions{Path: out, Encoder: enc}); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.Recording(); !on {
		t.Fatal("not recording")
	}
	time.Sleep(2 * time.Second)
	res, err := s.StopRecording(ctx)
	if err != nil || res.Partial {
		t.Fatalf("stop: %v %+v", err, res)
	}
	p, err := ProbeFile(ctx, ff, out)
	if err != nil {
		t.Fatal(err)
	}
	if !p.HasVideo || !p.HasAudio || p.Width != 640 || p.Height != 360 || p.VCodec != "h264" {
		t.Fatalf("probe %+v", p)
	}
	if p.Duration < 1500*time.Millisecond || p.Duration > 3500*time.Millisecond {
		t.Fatalf("duration %v for a 2 s recording", p.Duration)
	}
	// the preview came back after the recording
	drain(frames)
	select {
	case <-frames:
	case <-time.After(10 * time.Second):
		t.Fatal("preview did not resume after recording")
	}
	th, err := Thumbnail(ctx, ff, out, 500*time.Millisecond, 320)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := jpeg.DecodeConfig(bytes.NewReader(th)); err != nil || c.Width != 320 {
		t.Fatalf("thumb %v %+v", err, c)
	}
}

func drain(c chan []byte) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func TestMaxDurationEndsRecordingByItself(t *testing.T) {
	ff, enc := needFFmpeg(t)
	done := make(chan RecordResult, 1)
	s := NewStream(StreamOptions{FFmpeg: ff, Source: lavfiSource(), OnFrame: func([]byte) {},
		OnRecordEnd: func(r RecordResult) { done <- r }})
	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	out := filepath.Join(t.TempDir(), "max.mp4")
	if err := s.StartRecording(ctx, RecordOptions{Path: out, Encoder: enc, MaxDuration: 2 * time.Second}); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-done:
		if r.Reason != "max-duration" || r.Partial {
			t.Fatalf("%+v", r)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("max duration not enforced")
	}
	if p, err := ProbeFile(ctx, ff, out); err != nil || p.Duration < time.Second {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestKilledRecordingIsStillPlayable(t *testing.T) {
	ff, enc := needFFmpeg(t)
	lost := make(chan struct{}, 1)
	ended := make(chan RecordResult, 1)
	s := NewStream(StreamOptions{FFmpeg: ff, Source: lavfiSource(), OnFrame: func([]byte) {},
		OnLost: func(error, string) { lost <- struct{}{} }, OnRecordEnd: func(r RecordResult) { ended <- r }})
	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	out := filepath.Join(t.TempDir(), "crash.mp4")
	if err := s.StartRecording(ctx, RecordOptions{Path: out, Encoder: enc}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3500 * time.Millisecond)
	s.mu.Lock()
	proc := s.proc
	s.mu.Unlock()
	proc.Kill() // SIGKILL / TerminateProcess: simulates a crash or the device vanishing mid-recording
	select {
	case r := <-ended:
		if !r.Partial || r.Reason != "device-lost" {
			t.Fatalf("%+v", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("abnormal end not reported")
	}
	select {
	case <-lost:
	case <-time.After(5 * time.Second):
		t.Fatal("OnLost not called")
	}
	st, _ := os.Stat(out)
	if st == nil || st.Size() == 0 {
		t.Fatal("no partial file")
	}
	p, err := ProbeFile(ctx, ff, out)
	if err != nil || !p.HasVideo {
		t.Fatalf("partial recording not playable: %v %+v", err, p)
	}
}

// A driver that rejects the strict arguments (size, frame rate, format) must
// not make the camera unusable: opening walks down the attempt ladder.
func TestOpenFallsBackToSimplerArgumentsAndRemembersWhatWorked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a fake ffmpeg")
	}
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	var jb bytes.Buffer
	_ = jpeg.Encode(&jb, img, nil)
	fixture := filepath.Join(dir, "frame.jpg")
	_ = os.WriteFile(fixture, jb.Bytes(), 0o644)
	logf := filepath.Join(dir, "calls.log")
	script := filepath.Join(dir, "ffmpeg")
	_ = os.WriteFile(script, []byte(`#!/bin/bash
echo "$*" >> `+logf+`
case "$*" in
  *-video_size*|*-framerate*|*-input_format*) echo "Error opening input files: Invalid argument" >&2; exit 1;;
esac
while true; do
  cat `+fixture+`
  if read -r -n1 -t 0.05 _; then exit 0; fi
done
`), 0o755)

	frames := make(chan []byte, 16)
	s := NewStream(StreamOptions{FFmpeg: script, OnFrame: func(b []byte) {
		select {
		case frames <- b:
		default:
		}
	},
		Source: Source{Video: Device{Name: "USB2.0 HD IR UVC WebCam", ID: "/dev/video0"}, Width: 1280, Height: 720, FPS: 30, Format: "mjpeg"}})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("must fall back to the device default mode: %v", err)
	}
	select {
	case <-frames:
	case <-time.After(5 * time.Second):
		t.Fatal("no frames after the fallback")
	}
	if u := s.Used(); u.Width != 0 || u.Format != "" {
		t.Fatalf("the default-mode attempt should be in use: %+v", u)
	}
	calls, _ := os.ReadFile(logf)
	if n := strings.Count(string(calls), "\n"); n != 3 { // full, no-format, default
		t.Fatalf("expected 3 attempts, got %d:\n%s", n, calls)
	}
	s.Stop()

	// the next open starts with the variant that worked (one call, not three)
	_ = os.WriteFile(logf, nil, 0o644)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	calls, _ = os.ReadFile(logf)
	if n := strings.Count(string(calls), "\n"); n != 1 {
		t.Fatalf("the working variant must be tried first, got %d calls:\n%s", n, calls)
	}
}

func TestOpenFailureReportsEveryAttempt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a fake ffmpeg")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "ffmpeg")
	_ = os.WriteFile(script, []byte("#!/bin/sh\necho 'Could not run graph (sorry!)' >&2\nexit 1\n"), 0o755)
	s := NewStream(StreamOptions{FFmpeg: script, OnFrame: func([]byte) {},
		Source: Source{Video: Device{Name: "Cam", ID: "/dev/video0"}, Width: 640, Height: 480, FPS: 30, Format: "mjpeg"}})
	err := s.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "in any of") || !strings.Contains(err.Error(), "another program") {
		t.Fatalf("the error must list the attempts and the likely cause: %v", err)
	}
}

func TestAttemptsAreDistinctAndStrictestFirst(t *testing.T) {
	a := Source{Video: Device{Name: "Cam", ID: "/dev/video0"}, Width: 1280, Height: 720, FPS: 30, Format: "mjpeg"}.Attempts()
	if len(a) < 3 || a[0].Format != "mjpeg" || a[0].Width != 1280 {
		t.Fatalf("%+v", a)
	}
	seen := map[string]bool{}
	for _, x := range a {
		args, _ := x.inputArgs()
		k := strings.Join(args, " ")
		if seen[k] {
			t.Fatalf("duplicate attempt %s", k)
		}
		seen[k] = true
	}
	if last := a[len(a)-1]; last.Width != 0 {
		t.Fatalf("the ladder must end on the device default: %+v", last)
	}
	// an input override (tests) is never expanded
	if n := len((Source{InputOverride: []string{"-f", "lavfi", "-i", "x"}}).Attempts()); n != 1 {
		t.Fatal(n)
	}
}
