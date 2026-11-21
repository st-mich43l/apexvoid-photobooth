package video

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

// Mode is one capture format of a video device (shared with the camera package).
type Mode = camera.Mode

// Device is a capture device (camera or microphone).
type Device struct {
	Name  string `json:"name"`
	ID    string `json:"id"` // stable identifier (dshow alternative name, /dev/videoN, avfoundation index)
	Kind  string `json:"kind"`
	Modes []Mode `json:"modes,omitempty"`
	// USBID ("vid:pid") and Key (USB instance path) are derived from the
	// DirectShow alternative name when it carries one.
	USBID string `json:"usbId,omitempty"`
	Key   string `json:"key,omitempty"`
}

var reUSBPath = regexp.MustCompile(`(?i)usb#vid_([0-9a-f]{4})&pid_([0-9a-f]{4})(&mi_[0-9a-f]{2})?#([^#]+)#`)

// DeviceKeys derives the USB id ("04a9:32ea") and a stable key
// ("usb#vid_04a9&pid_32ea#<instance>") from a DirectShow alternative name /
// Windows device path such as
// `@device_pnp_\\?\usb#vid_04a9&pid_32ea#6&2a&0&2#{65e8773d-...}\global`.
// Both are empty when the path is not a USB path.
func DeviceKeys(path string) (usbID, key string) {
	m := reUSBPath.FindStringSubmatch(path)
	if m == nil {
		return "", ""
	}
	vid, pid, mi := strings.ToLower(m[1]), strings.ToLower(m[2]), strings.ToLower(m[3])
	// the interface (&mi_xx) stays in the key: a Windows Hello module is ONE usb
	// device with an RGB and an infrared video interface, which are two sources
	return vid + ":" + pid, "usb#vid_" + vid + "&pid_" + pid + mi + "#" + strings.ToLower(m[4])
}

// Devices is the result of a discovery.
type Devices struct {
	Video []Device `json:"video"`
	Audio []Device `json:"audio"`
}

var (
	reQuoted  = regexp.MustCompile(`"([^"]+)"\s*(\((video|audio|none)\))?`)
	reAlt     = regexp.MustCompile(`Alternative name "([^"]+)"`)
	reDShowOp = regexp.MustCompile(`(pixel_format|vcodec)=(\S+)\s+min s=(\d+)x(\d+) fps=([\d.]+) max s=(\d+)x(\d+) fps=([\d.]+)`)
	reAVF     = regexp.MustCompile(`\[(\d+)\]\s+(.+)$`)
)

// parseDShowDevices parses `ffmpeg -f dshow -list_devices true -i dummy`.
func parseDShowDevices(out string) Devices {
	var d Devices
	section := ""
	var last *Device
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		line = stripPrefix(line)
		switch {
		case strings.Contains(line, "DirectShow video devices"):
			section = "video"
			continue
		case strings.Contains(line, "DirectShow audio devices"):
			section = "audio"
			continue
		}
		if m := reAlt.FindStringSubmatch(line); m != nil {
			if last != nil {
				last.ID = m[1]
				last.USBID, last.Key = DeviceKeys(m[1])
			}
			continue
		}
		m := reQuoted.FindStringSubmatch(line)
		if m == nil || strings.HasPrefix(line, "Could not") {
			continue
		}
		kind := section
		if m[3] == "video" || m[3] == "audio" {
			kind = m[3]
		}
		if kind == "" {
			continue
		}
		dev := Device{Name: m[1], ID: m[1], Kind: kind}
		if kind == "video" {
			d.Video = append(d.Video, dev)
			last = &d.Video[len(d.Video)-1]
		} else {
			d.Audio = append(d.Audio, dev)
			last = &d.Audio[len(d.Audio)-1]
		}
	}
	return d
}

func stripPrefix(l string) string {
	if strings.HasPrefix(l, "[") {
		if i := strings.Index(l, "] "); i >= 0 {
			return strings.TrimSpace(l[i+2:])
		}
	}
	return l
}

// parseDShowModes parses `-list_options true`.
func parseDShowModes(out string) []Mode {
	var modes []Mode
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		m := reDShowOp.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		w, _ := strconv.Atoi(m[6])
		h, _ := strconv.Atoi(m[7])
		fmin, _ := strconv.ParseFloat(m[5], 64)
		fmax, _ := strconv.ParseFloat(m[8], 64)
		key := m[1] + m[2] + m[6] + "x" + m[7]
		if seen[key] {
			continue
		}
		seen[key] = true
		modes = append(modes, Mode{Width: w, Height: h, FPSMin: fmin, FPSMax: fmax, Format: m[2]})
	}
	sortModes(modes)
	return modes
}

func sortModes(m []Mode) {
	sort.SliceStable(m, func(i, j int) bool {
		if m[i].Width*m[i].Height != m[j].Width*m[j].Height {
			return m[i].Width*m[i].Height > m[j].Width*m[j].Height
		}
		return m[i].FPSMax > m[j].FPSMax
	})
}

// parseV4LModes parses `-f v4l2 -list_formats all`.
func parseV4LModes(out string) []Mode {
	var modes []Mode
	for _, line := range strings.Split(out, "\n") {
		line = stripPrefix(strings.TrimSpace(line))
		if !strings.HasPrefix(line, "Raw") && !strings.HasPrefix(line, "Compressed") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}
		format := strings.TrimSpace(parts[1])
		for _, sz := range strings.Fields(parts[len(parts)-1]) {
			wh := strings.SplitN(sz, "x", 2)
			if len(wh) != 2 {
				continue
			}
			w, e1 := strconv.Atoi(wh[0])
			h, e2 := strconv.Atoi(wh[1])
			if e1 == nil && e2 == nil {
				modes = append(modes, Mode{Width: w, Height: h, Format: format})
			}
		}
	}
	sortModes(modes)
	return modes
}

// parseAVFDevices parses macOS `-f avfoundation -list_devices true -i ""`.
func parseAVFDevices(out string) Devices {
	var d Devices
	section := ""
	for _, raw := range strings.Split(out, "\n") {
		line := stripPrefix(strings.TrimSpace(raw))
		if strings.Contains(line, "AVFoundation video devices") {
			section = "video"
			continue
		}
		if strings.Contains(line, "AVFoundation audio devices") {
			section = "audio"
			continue
		}
		if m := reAVF.FindStringSubmatch(line); m != nil && section != "" {
			dev := Device{Name: strings.TrimSpace(m[2]), ID: m[1], Kind: section}
			if section == "video" {
				d.Video = append(d.Video, dev)
			} else {
				d.Audio = append(d.Audio, dev)
			}
		}
	}
	return d
}

// ListDevices discovers capture devices on this platform through FFmpeg.
// Windows (DirectShow) is the validated path; Linux (V4L2) is implemented and
// exercised in CI-less development; macOS (AVFoundation) is parse-only.
func ListDevices(ctx context.Context, ffmpeg string, withModes bool) (Devices, error) {
	switch runtime.GOOS {
	case "windows":
		out, _ := run(ctx, ffmpeg, 15*time.Second, "-list_devices", "true", "-f", "dshow", "-i", "dummy")
		d := parseDShowDevices(out)
		if withModes {
			for i := range d.Video {
				mo, _ := run(ctx, ffmpeg, 15*time.Second, "-list_options", "true", "-f", "dshow", "-i", "video="+d.Video[i].Name)
				d.Video[i].Modes = parseDShowModes(mo)
			}
		}
		return d, nil
	case "darwin":
		out, _ := run(ctx, ffmpeg, 15*time.Second, "-f", "avfoundation", "-list_devices", "true", "-i", "")
		return parseAVFDevices(out), nil
	default:
		var d Devices
		paths, _ := filepath.Glob("/dev/video*")
		sort.Strings(paths)
		for _, p := range paths {
			out, _ := run(ctx, ffmpeg, 10*time.Second, "-f", "v4l2", "-list_formats", "all", "-i", p)
			modes := parseV4LModes(out)
			if len(modes) == 0 {
				continue // metadata node or not a capture device
			}
			name := filepath.Base(p)
			if b, err := os.ReadFile("/sys/class/video4linux/" + filepath.Base(p) + "/name"); err == nil {
				name = strings.TrimSpace(string(b))
			}
			d.Video = append(d.Video, Device{Name: name, ID: p, Kind: "video", Modes: modes})
		}
		d.Audio = []Device{{Name: "Default microphone (PulseAudio/PipeWire)", ID: "default", Kind: "audio"}}
		return d, nil
	}
}
