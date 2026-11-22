package edsdk

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

// DriverName is the camera.DeviceInfo.Driver value.
const DriverName = "canon-edsdk"

// Driver discovers and opens Canon cameras through EDSDK.
type Driver struct {
	// SDKDir returns the configured directory containing EDSDK.dll ("" = search).
	SDKDir func() string

	newAPI func(dir string) (api, error) // injectable; default = real DLL
	find   conflictFinder

	mu sync.Mutex
	a  api
	w  *worker
}

// NewDriver creates the driver. sdkDir may be nil.
func NewDriver(sdkDir func() string) *Driver {
	return &Driver{SDKDir: sdkDir, newAPI: loadDLL, find: runningProcessNames}
}

func (d *Driver) Name() string { return DriverName }

func (d *Driver) dir() string {
	if d.SDKDir != nil {
		return d.SDKDir()
	}
	return ""
}

// Available reports whether the Canon SDK can be loaded (Windows + EDSDK.dll
// present). It does not initialise the SDK.
func (d *Driver) Available() error {
	d.mu.Lock()
	ready := d.w != nil
	d.mu.Unlock()
	if ready {
		return nil
	}
	return sdkAvailable(d.dir())
}

// ensure starts the SDK thread on first use.
func (d *Driver) ensure() (*worker, api, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.w != nil {
		return d.w, d.a, nil
	}
	a, err := d.newAPI(d.dir())
	if err != nil {
		return nil, nil, err
	}
	w, err := startWorker(a)
	if err != nil {
		return nil, nil, camera.Errf(camera.CodeSDKMissing, "Reinstall the Canon EDSDK files (EDSDK.dll and EdsImage.dll, x64) next to the application.", err, "Canon EDSDK failed to initialise")
	}
	d.w, d.a = w, a
	return w, a, nil
}

// ShutdownTimeout bounds how long Close waits for the SDK thread.
var ShutdownTimeout = 10 * time.Second

// Close terminates the SDK (call on application exit, after cameras are
// disconnected). It is bounded: a stuck SDK call is reported, not waited on forever.
func (d *Driver) Close() error {
	d.mu.Lock()
	w := d.w
	d.w, d.a = nil, nil
	d.mu.Unlock()
	if w == nil {
		return nil
	}
	return w.stop(ShutdownTimeout)
}

// Health reports the SDK thread state ("not started" until first use).
func (d *Driver) Health() (Health, bool) {
	d.mu.Lock()
	w := d.w
	d.mu.Unlock()
	if w == nil {
		return Health{}, false
	}
	return w.Health(), true
}

// stableIDs derives an identity from the USB port path the SDK reports (which
// contains the vendor/product ids and the device instance), never from the
// position in the list, so reordering or hot-plugging another camera does not
// change it. Only entries that report the very same key (two identical bodies
// that expose no distinguishing port) get an occurrence suffix.
func stableIDs(cams []rawCamera) []string {
	ids := make([]string, len(cams))
	seen := map[string]int{}
	for i, c := range cams {
		key := strings.TrimSpace(c.Port)
		if key == "" {
			key = "name:" + strings.TrimSpace(c.Name)
		}
		id := DriverName + ":" + key
		if n := seen[id]; n > 0 {
			id = fmt.Sprintf("%s#%d", id, n+1)
		}
		seen[DriverName+":"+key]++
		ids[i] = id
	}
	return ids
}

var usbIDRe = regexp.MustCompile(`(?i)vid_([0-9a-f]{4})&pid_([0-9a-f]{4})`)

// usbIDOf extracts "vid:pid" (lower-case hex) from an EDSDK port string.
func usbIDOf(port string) string {
	if m := usbIDRe.FindStringSubmatch(port); m != nil {
		return strings.ToLower(m[1] + ":" + m[2])
	}
	return ""
}

// list reads the camera list and releases all handles unless keep matches.
func (d *Driver) list(ctx context.Context, keepID string) ([]camera.DeviceInfo, rawCamera, error) {
	w, a, err := d.ensure()
	if err != nil {
		return nil, rawCamera{}, err
	}
	var infos []camera.DeviceInfo
	var kept rawCamera
	err = w.do(ctx, func() error {
		cams, err := a.CameraList()
		if err != nil {
			return err
		}
		ids := stableIDs(cams)
		for i, c := range cams {
			id := ids[i]
			infos = append(infos, camera.DeviceInfo{ID: id, Name: c.Name, Driver: DriverName, Port: c.Port, Kind: camera.KindDSLR, Vendor: "Canon",
				USBID: usbIDOf(c.Port), Interface: "USB (Canon EDSDK)"})
			if keepID != "" && id == keepID {
				kept = c
			} else {
				a.Release(c.Ref)
			}
		}
		return nil
	})
	if err != nil {
		return nil, rawCamera{}, translate("discover cameras", err, d.find)
	}
	return infos, kept, nil
}

// Discover lists connected Canon cameras.
func (d *Driver) Discover(ctx context.Context) ([]camera.DeviceInfo, error) {
	infos, _, err := d.list(ctx, "")
	return infos, err
}

// Open binds a Camera to a discovered device (no session yet).
func (d *Driver) Open(info camera.DeviceInfo) (camera.Camera, error) {
	w, a, err := d.ensure()
	if err != nil {
		return nil, err
	}
	return newCamera(w, a, info, d.find, d), nil
}
