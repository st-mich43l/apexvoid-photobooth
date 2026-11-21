package edsdk

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// headerValue finds NAME in C header text, accepting
//
//	#define NAME 0x0000000b          (EDSDKErrors.h, some types)
//	NAME = 0x0000000b,               (enums in EDSDKTypes.h)
func headerValue(text, name string) (uint64, bool) {
	re := regexp.MustCompile(`(?m)(?:#define\s+` + regexp.QuoteMeta(name) + `\s+|\b` + regexp.QuoteMeta(name) + `\s*=\s*)\(?\s*(0[xX][0-9A-Fa-f]+|\d+)[uUlL]*`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseUint(m[1], 0, 64)
	return v, err == nil
}

// constantsToVerify maps the header symbol to the value this adapter uses.
func constantsToVerify() map[string]uint64 {
	return map[string]uint64{
		"kEdsPropID_ProductName":                           propProductName,
		"kEdsPropID_ImageQuality":                          propImageQuality,
		"kEdsPropID_SaveTo":                                propSaveTo,
		"kEdsPropID_Evf_OutputDevice":                      propEvfOutputDevice,
		"kEdsPropID_Evf_Mode":                              propEvfMode,
		"kEdsSaveTo_Host":                                  saveToHost,
		"kEdsEvfOutputDevice_PC":                           evfOutputPC,
		"kEdsCameraCommand_TakePicture":                    cmdTakePicture,
		"kEdsCameraCommand_ExtendShutDownTimer":            cmdExtendShutdownTimer,
		"kEdsCameraCommand_PressShutterButton":             cmdPressShutter,
		"kEdsCameraCommand_ShutterButton_OFF":              shutterOff,
		"kEdsCameraCommand_ShutterButton_Completely_NonAF": shutterCompletelyNonAF,
		"kEdsObjectEvent_DirItemCreated":                   evObjectDirItemCreated,
		"kEdsObjectEvent_DirItemRequestTransfer":           evObjectDirItemRequestTransfer,
		"kEdsStateEvent_Shutdown":                          evStateShutdown,
		"kEdsStateEvent_WillSoonShutDown":                  evStateWillSoonShutDown,
		"EDS_ERR_UNIMPLEMENTED":                            errUnimplemented,
		"EDS_ERR_NOT_SUPPORTED":                            errNotSupported,
		"EDS_ERR_DEVICE_NOT_FOUND":                         errDeviceNotFound,
		"EDS_ERR_DEVICE_BUSY":                              errDeviceBusy,
		"EDS_ERR_DEVICE_INVALID":                           errDeviceInvalid,
		"EDS_ERR_DEVICE_MEMORY_FULL":                       errDeviceMemoryFull,
		"EDS_ERR_DEVICE_NO_DISK":                           errDeviceNoDisk,
		"EDS_ERR_COMM_PORT_IS_IN_USE":                      errCommPortInUse,
		"EDS_ERR_COMM_DISCONNECTED":                        errCommDisconnected,
		"EDS_ERR_COMM_DEVICE_INCOMPATIBLE":                 errCommDeviceIncompat,
		"EDS_ERR_COMM_USB_BUS_ERR":                         errCommUSBBus,
		"EDS_ERR_USB_DEVICE_LOCK_ERROR":                    errUSBLockError,
		"EDS_ERR_OBJECT_NOTREADY":                          errObjectNotReady,
		"EDS_ERR_TAKE_PICTURE_AF_NG":                       errTakePictureAFNG,
		"EDS_ERR_TAKE_PICTURE_NO_CARD_NG":                  errTakePictureNoCard,
		"EDS_ERR_TAKE_PICTURE_CARD_NG":                     errTakePictureCardNG,
		"EDS_ERR_TAKE_PICTURE_CARD_PROTECT_NG":             errTakePictureProtect,
	}
}

// verifyAgainstHeaders returns one line per mismatch / missing symbol.
func verifyAgainstHeaders(text string) []string {
	var bad []string
	for name, ours := range constantsToVerify() {
		theirs, ok := headerValue(text, name)
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("%s: not found in the headers", name))
		case theirs != ours:
			bad = append(bad, fmt.Sprintf("%s: header says 0x%X, adapter uses 0x%X", name, theirs, ours))
		}
	}
	return bad
}

// TestConstantsMatchCanonHeaders checks every constant this adapter uses
// against the headers of the SDK you installed. Canon's headers are
// proprietary and not in this repository, so the test only runs when pointed
// at them:
//
//	EDSDK_HEADERS=C:\Canon\EDSDK\Windows\EDSDK\Header go test ./internal/camera/edsdk -run Headers -v
func TestConstantsMatchCanonHeaders(t *testing.T) {
	dir := os.Getenv("EDSDK_HEADERS")
	if dir == "" {
		t.Skip("set EDSDK_HEADERS to the folder containing EDSDKTypes.h and EDSDKErrors.h to verify the constants")
	}
	var text strings.Builder
	for _, f := range []string{"EDSDKTypes.h", "EDSDKErrors.h", "EDSDK.h"} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			if f == "EDSDK.h" {
				continue
			}
			t.Fatalf("cannot read %s: %v", f, err)
		}
		text.Write(b)
		text.WriteByte('\n')
	}
	if bad := verifyAgainstHeaders(text.String()); len(bad) > 0 {
		t.Fatalf("the adapter disagrees with the installed SDK headers:\n  %s", strings.Join(bad, "\n  "))
	}
}

// The verifier itself is tested against a miniature header in the two formats
// Canon uses, including a deliberately wrong value.
func TestHeaderVerifierDetectsMismatchAndMissingSymbols(t *testing.T) {
	var b strings.Builder
	for name, v := range constantsToVerify() {
		if strings.HasPrefix(name, "EDS_ERR") {
			fmt.Fprintf(&b, "#define %s 0x%08XL\n", name, v)
		} else {
			fmt.Fprintf(&b, "    %s = 0x%08X,\n", name, v)
		}
	}
	good := b.String()
	if bad := verifyAgainstHeaders(good); len(bad) != 0 {
		t.Fatalf("a matching header must verify cleanly: %v", bad)
	}
	wrong := strings.Replace(good, fmt.Sprintf("kEdsPropID_SaveTo = 0x%08X,", propSaveTo), "kEdsPropID_SaveTo = 0x00000099,", 1)
	bad := verifyAgainstHeaders(wrong)
	if len(bad) != 1 || !strings.Contains(bad[0], "kEdsPropID_SaveTo") || !strings.Contains(bad[0], "0x99") {
		t.Fatalf("%v", bad)
	}
	missing := strings.Replace(good, "EDS_ERR_DEVICE_BUSY", "EDS_ERR_SOMETHING_ELSE", 1)
	if bad := verifyAgainstHeaders(missing); len(bad) != 1 || !strings.Contains(bad[0], "not found") {
		t.Fatalf("%v", bad)
	}
}
