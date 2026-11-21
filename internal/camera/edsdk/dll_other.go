//go:build !windows

package edsdk

import "github.com/st-mich43l/apexvoid-photobooth/internal/camera"

func sdkAvailable(string) error {
	return camera.Errf(camera.CodePlatform, "Canon EDSDK support is implemented for Windows x64 only (macOS/Linux are future targets).", nil, "Canon EDSDK is not available on this platform")
}

func loadDLL(string) (api, error) { return nil, sdkAvailable("") }

func runningProcessNames() []string { return nil }
