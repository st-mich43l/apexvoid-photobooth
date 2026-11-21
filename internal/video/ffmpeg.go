// Package video wraps FFmpeg for webcam/UVC capture: device discovery,
// a live-preview + MP4 recording pipeline, and probing. FFmpeg is an external
// program located at runtime (not linked, not committed); see docs/CAMERAS.md.
package video

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrFFmpegMissing is returned when no ffmpeg executable can be found.
var ErrFFmpegMissing = errors.New("ffmpeg executable not found")

// exeName is the platform executable name.
func exeName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

// Locate finds ffmpeg: the configured path, then next to the application
// executable (portable install: ffmpeg.exe beside the app, or in ./ffmpeg/),
// then PATH.
func Locate(configured string) (string, error) {
	var cands []string
	if configured != "" {
		cands = append(cands, configured)
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands, filepath.Join(d, exeName()), filepath.Join(d, "ffmpeg", exeName()), filepath.Join(d, "ffmpeg", "bin", exeName()))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	return "", ErrFFmpegMissing
}

// run executes ffmpeg and returns combined stderr+stdout (ffmpeg prints
// device lists to stderr).
func run(ctx context.Context, ffmpeg string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, append([]string{"-hide_banner", "-nostdin"}, args...)...)
	hide(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// H264Encoders in order of preference. libx264 needs a GPL FFmpeg build;
// h264_mf (Windows Media Foundation) works with LGPL builds; the hardware
// encoders need the matching GPU.
var H264Encoders = []string{"libx264", "h264_mf", "h264_nvenc", "h264_qsv", "h264_amf", "libopenh264"}

// PickEncoder returns the first usable H.264 encoder reported by ffmpeg.
func PickEncoder(ctx context.Context, ffmpeg string) (string, error) {
	out, err := run(ctx, ffmpeg, 10*time.Second, "-encoders")
	if err != nil {
		return "", fmt.Errorf("ffmpeg -encoders: %w", err)
	}
	return pickEncoder(out)
}

func pickEncoder(listing string) (string, error) {
	have := map[string]bool{}
	for _, line := range strings.Split(listing, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && len(f[0]) == 6 && f[0][0] == 'V' {
			have[f[1]] = true
		}
	}
	for _, e := range H264Encoders {
		if have[e] {
			return e, nil
		}
	}
	return "", errors.New("this ffmpeg build has no H.264 encoder (libx264, h264_mf, nvenc, qsv, amf)")
}

// Probe describes a media file.
type Probe struct {
	Duration time.Duration
	Width    int
	Height   int
	HasVideo bool
	HasAudio bool
	VCodec   string
	ACodec   string
}

// ProbeFile inspects a media file using `ffmpeg -i` (no ffprobe needed).
func ProbeFile(ctx context.Context, ffmpeg, path string) (Probe, error) {
	out, _ := run(ctx, ffmpeg, 20*time.Second, "-i", path) // exits 1 without an output file; that's expected
	p := parseProbe(out)
	if !p.HasVideo && !p.HasAudio {
		return p, fmt.Errorf("not a readable media file: %s", firstLine(out))
	}
	return p, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Thumbnail extracts a poster frame as JPEG.
func Thumbnail(ctx context.Context, ffmpeg, path string, at time.Duration, maxWidth int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-ss", fmt.Sprintf("%.3f", at.Seconds()), "-i", path,
		"-frames:v", "1", "-vf", fmt.Sprintf("scale='min(%d,iw)':-2", maxWidth), "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "4", "pipe:1")
	hide(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		return nil, fmt.Errorf("thumbnail: %v %s", err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}
