package edsdk

import (
	"errors"
	"strings"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

// conflictFinder lists known programs that take exclusive ownership of a
// Canon camera. Injected so it is testable; the Windows build uses Toolhelp.
type conflictFinder func() []string

// canonUSBOwners are processes that commonly hold the camera's USB session.
var canonUSBOwners = []string{
	"EOS Utility 3.exe", "EOS Utility.exe", "EOSUtility.exe", "EOS Webcam Utility Pro.exe", "EOSWebcamUtilityPro.exe",
	"CameraWindow.exe", "CameraWindowDC.exe", "DPP4.exe", "ZoomBrowser.exe", "ImageBrowser.exe",
}

// matchOwners returns the running processes (case-insensitive) that are known
// to own Canon cameras.
func matchOwners(running []string) []string {
	var out []string
	for _, r := range running {
		for _, k := range canonUSBOwners {
			if strings.EqualFold(r, k) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// translate converts an EDSDK failure into an actionable camera.Error.
func translate(op string, err error, find conflictFinder) error {
	if err == nil {
		return nil
	}
	var ce *camera.Error
	if errors.As(err, &ce) {
		return err
	}
	var se *sdkError
	if !errors.As(err, &se) {
		return err
	}
	e := func(code, action, msg string) error { return camera.Errf(code, action, se, "%s", msg) }
	switch se.Code {
	case errDeviceBusy, errCommPortInUse, errUSBLockError:
		action := "Close EOS Utility, EOS Webcam Utility, Canon Camera Connect, Windows Photos/Camera import and any other program using the camera, then reconnect."
		if find != nil {
			if names := matchOwners(find()); len(names) > 0 {
				action = "Close " + strings.Join(names, ", ") + " (it owns the camera's USB connection), then reconnect."
			}
		}
		return e(camera.CodeDeviceBusy, action, op+": the camera is busy or owned by another program")
	case errDeviceNotFound, errCommDisconnected, errDeviceInvalid, errCommUSBBus:
		return e(camera.CodeDisconnected, "Check the USB cable and that the camera is switched on (and not asleep). The app reconnects automatically.", op+": the camera was disconnected")
	case errCommDeviceIncompat:
		return e(camera.CodeUnsupported, "This camera model is not supported by the installed EDSDK version. Install a newer SDK build (see docs/CAMERAS.md).", op+": camera not supported by this EDSDK version")
	case errNotSupported, errUnimplemented:
		return e(camera.CodeUnsupported, "The camera or SDK version does not support this operation.", op+": not supported")
	case errObjectNotReady:
		return e(camera.CodeCaptureFailed, "The camera is not ready (focusing or busy). Wait a moment and retry.", op+": camera not ready")
	case errTakePictureAFNG:
		return e(camera.CodeCaptureFailed, "Autofocus failed. Check focus/lighting, or switch the lens to MF, then retry.", op+": autofocus failed")
	case errTakePictureNoCard, errTakePictureCardNG, errTakePictureProtect, errDeviceNoDisk:
		return e(camera.CodeCaptureFailed, "Check the memory card (inserted, not full, not write-protected).", op+": memory card problem")
	case errDeviceMemoryFull:
		return e(camera.CodeCaptureFailed, "The camera memory/card is full.", op+": camera memory full")
	}
	return e(camera.CodeCaptureFailed, "Check that the camera is in Photo mode (not Movie) with a lens attached, then retry.", op+": camera reported an error")
}

func sdkCode(err error) (uint32, bool) {
	var se *sdkError
	if errors.As(err, &se) {
		return se.Code, true
	}
	return 0, false
}

// Conflicts lists running programs known to hold a Canon camera's USB session.
func Conflicts() []string { return matchOwners(runningProcessNames()) }
