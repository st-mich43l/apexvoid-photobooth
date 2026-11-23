//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows/registry"
)

// Microsoft documents checking the Evergreen runtime registration in both
// per-machine (32-bit registry view on x64) and per-user locations.
func checkWebViewRuntime() error {
	const key = `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	for _, location := range []struct {
		root   registry.Key
		access uint32
	}{
		{registry.LOCAL_MACHINE, registry.QUERY_VALUE | registry.WOW64_32KEY},
		{registry.CURRENT_USER, registry.QUERY_VALUE},
	} {
		k, err := registry.OpenKey(location.root, key, location.access)
		if err != nil {
			continue
		}
		version, _, err := k.GetStringValue("pv")
		_ = k.Close()
		if err == nil && validRuntimeVersion(version) {
			return nil
		}
	}
	return errors.New("Microsoft Edge WebView2 Evergreen Runtime was not detected. Install the WebView2 Evergreen Runtime, then reopen ApexVoid. For offline kiosks, use Microsoft's x64 Evergreen Standalone Installer. See docs/INSTALL.md.")
}
