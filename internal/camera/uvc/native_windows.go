//go:build windows

package uvc

import (
	"context"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows PnP device classes that hold cameras.
var (
	classCamera = windows.GUID{Data1: 0xca3e7ab9, Data2: 0xb4c3, Data3: 0x4ae6, Data4: [8]byte{0x82, 0x51, 0x57, 0x9e, 0xf9, 0x33, 0x89, 0x0f}}
	classImage  = windows.GUID{Data1: 0x6bdd1fc6, Data2: 0x810f, Data3: 0x11d0, Data4: [8]byte{0xbe, 0xc7, 0x08, 0x00, 0x2b, 0xe2, 0x09, 0x2b}}
)

// DirectShow "Video Input Device" category: software cameras register here.
const dshowVideoInputKey = `CLSID\{860BB310-5D01-11D0-BD3B-00A0C911CE86}\Instance`

// nativeEnumerator lists cameras without FFmpeg:
//   - PnP devices of class Camera and Image that are present (physical webcams,
//     built-in laptop cameras, UVC capture cards, a Canon body in UVC mode);
//   - DirectShow software filters from the registry (virtual cameras).
//
// Media Foundation (MFEnumDeviceSources) was evaluated and not used: it needs
// COM vtable calls from Go that cannot be exercised without Windows hardware;
// SetupAPI + registry give names, USB ids and stable instance keys with plain
// syscalls. Capture/preview still go through FFmpeg/DirectShow.
var nativeEnumerator Enumerator = func(ctx context.Context) ([]NativeDevice, error) {
	var out []NativeDevice
	seen := map[string]bool{}
	for _, class := range []windows.GUID{classCamera, classImage} {
		devs, err := pnpPresent(&class)
		if err != nil {
			continue
		}
		for _, d := range devs {
			if !seen[strings.ToLower(d.InstanceID)] {
				seen[strings.ToLower(d.InstanceID)] = true
				out = append(out, d)
			}
		}
	}
	// only USB-ish PnP devices are cameras in the Image class (scanners, phones also appear there)
	filtered := out[:0]
	for _, d := range out {
		if strings.HasPrefix(strings.ToUpper(d.InstanceID), "USB\\") || strings.HasPrefix(strings.ToUpper(d.InstanceID), "ACPI\\") || strings.HasPrefix(strings.ToUpper(d.InstanceID), "SWD\\") || strings.HasPrefix(strings.ToUpper(d.InstanceID), "ROOT\\") {
			filtered = append(filtered, d)
		}
	}
	out = filtered
	for _, name := range softwareCameras() {
		dup := false
		for _, d := range out {
			if strings.EqualFold(d.Name, name) {
				dup = true
			}
		}
		if !dup {
			out = append(out, NativeDevice{Name: name, Virtual: true})
		}
	}
	return out, nil
}

func pnpPresent(class *windows.GUID) ([]NativeDevice, error) {
	set, err := windows.SetupDiGetClassDevsEx(class, "", 0, windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return nil, err
	}
	defer windows.SetupDiDestroyDeviceInfoList(set)
	var out []NativeDevice
	for i := 0; ; i++ {
		info, err := windows.SetupDiEnumDeviceInfo(set, i)
		if err != nil {
			break // ERROR_NO_MORE_ITEMS
		}
		id, err := windows.SetupDiGetDeviceInstanceId(set, info)
		if err != nil {
			continue
		}
		name := ""
		if v, err := windows.SetupDiGetDeviceRegistryProperty(set, info, windows.SPDRP_FRIENDLYNAME); err == nil {
			name, _ = v.(string)
		}
		if name == "" {
			if v, err := windows.SetupDiGetDeviceRegistryProperty(set, info, windows.SPDRP_DEVICEDESC); err == nil {
				name, _ = v.(string)
			}
		}
		if name != "" {
			out = append(out, NativeDevice{Name: name, InstanceID: id})
		}
	}
	return out, nil
}

func softwareCameras() []string {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, dshowVideoInputKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	var out []string
	for _, n := range names {
		sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		if fn, _, err := sk.GetStringValue("FriendlyName"); err == nil && fn != "" {
			// physical cameras are not registered here; entries with a DevicePath are real hardware
			if _, _, err := sk.GetStringValue("DevicePath"); err != nil {
				out = append(out, fn)
			}
		}
		sk.Close()
	}
	return out
}
