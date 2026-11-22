//go:build !windows

package edsdk

import "github.com/st-mich43l/apexvoid-photobooth/internal/camera"

func sdkAvailable(string) error {
	return camera.Errf(camera.CodePlatform, "Canon EDSDK support is implemented for Windows x64 only (macOS/Linux are future targets).", nil, "Canon EDSDK is not available on this platform")
}

func loadDLL(string) (api, error) { return nil, sdkAvailable("") }

func runningProcessNames() []string { return nil }

// resolveDependency cannot check the Windows DLL search path on other platforms.
func resolveDependency(name, dir string) bool { return true }

// SDKStatus statically inspects the Canon SDK folder; off Windows the SDK is not usable.
func SDKStatus(configured string) SDKReport {
	return SDKReport{Kind: SDKPlatform, Problem: "Canon EDSDK support is implemented for Windows x64 only (macOS/Linux are future targets).",
		Action: "Use a Windows 10/11 x64 machine for Canon cameras. Webcams keep working."}
}
