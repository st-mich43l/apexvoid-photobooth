package uvc

import (
	"context"
	"regexp"
	"strings"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

// NativeDevice is a video device found through the operating system without
// FFmpeg (Windows: PnP "Camera"/"Image" class devices via SetupAPI, plus the
// DirectShow software-camera registry for virtual cameras).
type NativeDevice struct {
	Name       string
	InstanceID string // PnP instance id, e.g. USB\VID_04A9&PID_32EA&MI_00\6&2A&0&2 ("" for software cameras)
	Virtual    bool   // DirectShow software filter (OBS Virtual Camera, ...)
}

// Enumerator lists devices natively. nil means "not available on this platform".
type Enumerator func(ctx context.Context) ([]NativeDevice, error)

// keys derives (usbID, key) from a PnP instance id so it can be matched with a
// DirectShow alternative name for the same device.
func (n NativeDevice) keys() (usbID, key string) {
	if n.InstanceID == "" {
		return "", ""
	}
	p := `@device_pnp_\\?\` + strings.ToLower(strings.ReplaceAll(n.InstanceID, `\`, "#")) + `#{x}`
	return video.DeviceKeys(p)
}

var reCaptureCard = regexp.MustCompile(`(?i)cam ?link|elgato|avermedia|magewell|blackmagic|decklink|ezcap|capture|hdmi|video grabber|usb ?3\.0 video`)
var reVirtual = regexp.MustCompile(`(?i)virtual|obs|ndi|manycam|xsplit|snap camera|camo|iriun|droidcam|epoccam|eos webcam utility|vcam|splitcam`)

// Classify decides the source category and vendor from the name and USB id.
// It is a heuristic and is only used for labelling.
func Classify(name, usbID string, softwareFilter bool) (kind, vendor string) {
	n := strings.ToLower(name)
	canon := strings.HasPrefix(usbID, "04a9:") || strings.Contains(n, "canon") || strings.Contains(n, "eos")
	if canon {
		vendor = "Canon"
	}
	switch {
	case reVirtual.MatchString(n) || (softwareFilter && usbID == ""):
		if strings.Contains(n, "eos webcam utility") {
			vendor = "Canon"
		}
		return camera.KindVirtual, vendor
	case canon && strings.HasPrefix(usbID, "04a9:"):
		return camera.KindCanonUVC, vendor
	case reCaptureCard.MatchString(n):
		return camera.KindCaptureCard, vendor
	}
	return camera.KindWebcam, vendor
}

// mergeDevices combines the native list (always available, no FFmpeg) with the
// DirectShow/V4L2 list from FFmpeg (adds resolutions and the exact names FFmpeg
// needs). Entries are matched by USB instance key, then by name.
func mergeDevices(native []NativeDevice, ff []video.Device) []video.Device {
	used := make([]bool, len(ff))
	var out []video.Device
	for _, n := range native {
		usb, key := n.keys()
		match := -1
		for i, f := range ff {
			if used[i] {
				continue
			}
			if key != "" && f.Key == key {
				match = i
				break
			}
		}
		if match < 0 {
			for i, f := range ff { // fall back to the name
				if !used[i] && strings.EqualFold(f.Name, n.Name) {
					match = i
					break
				}
			}
		}
		if match >= 0 {
			used[match] = true
			d := ff[match]
			if d.USBID == "" {
				d.USBID, d.Key = usb, key
			}
			out = append(out, d)
			continue
		}
		out = append(out, video.Device{Name: n.Name, ID: n.Name, Kind: "video", USBID: usb, Key: key, Modes: nil})
	}
	for i, f := range ff { // DirectShow-only entries: virtual cameras, or all of them without native enumeration
		if !used[i] {
			out = append(out, f)
		}
	}
	return out
}

// stableIDs assigns IDs that survive refresh and unplug/replug: the USB
// instance key when known, otherwise the normalised name; identical names get
// a numeric suffix in discovery order.
func stableIDs(devs []video.Device) []string {
	ids := make([]string, len(devs))
	seen := map[string]int{}
	for i, d := range devs {
		base := d.Key
		if base == "" {
			base = "name:" + strings.ToLower(strings.TrimSpace(d.Name))
		}
		seen[base]++
		if n := seen[base]; n > 1 {
			base += "#" + string(rune('0'+n))
		}
		ids[i] = DriverName + ":" + base
	}
	return ids
}
