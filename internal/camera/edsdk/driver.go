package edsdk

import (
	"context"
	"fmt"
	"sync"

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

// Close terminates the SDK (call on application exit, after cameras are disconnected).
func (d *Driver) Close() error {
	d.mu.Lock()
	w := d.w
	d.w, d.a = nil, nil
	d.mu.Unlock()
	if w != nil {
		w.stop()
	}
	return nil
}

func deviceID(c rawCamera, idx int) string {
	return fmt.Sprintf("%s:%s#%d", DriverName, c.Port, idx)
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
		for i, c := range cams {
			id := deviceID(c, i)
			infos = append(infos, camera.DeviceInfo{ID: id, Name: c.Name, Driver: DriverName, Port: c.Port})
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
