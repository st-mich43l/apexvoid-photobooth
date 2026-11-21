// Package edsdk is the Canon EDSDK adapter (primary target: EOS R50 over USB
// on Windows 10/11 x64). It loads Canon's own EDSDK.dll at runtime; the SDK is
// proprietary and is NOT part of this repository or the release artifact
// (see docs/CAMERAS.md). All SDK calls run on one dedicated OS thread that
// also pumps the Windows message queue, as the SDK requires.
//
// Constant values and struct layouts below were written from Canon's public
// EDSDK headers and MUST be re-checked against the headers of the SDK version
// you install (EDSDKTypes.h / EDSDKErrors.h) during hardware validation.
package edsdk

import "fmt"

// Object / state event ids.
const (
	evObjectDirItemCreated         = 0x00000204
	evObjectDirItemRequestTransfer = 0x00000208
	evStateShutdown                = 0x00000301
	evStateWillSoonShutDown        = 0x00000303
)

// Property ids.
const (
	propProductName     = 0x00000002
	propImageQuality    = 0x00000100
	propSaveTo          = 0x0000000b
	propEvfOutputDevice = 0x00000500
	propEvfMode         = 0x00000501
)

// Values.
const (
	saveToHost             = 2
	evfOutputPC            = 2
	cmdTakePicture         = 0x00000000
	cmdExtendShutdownTimer = 0x00000001
	cmdPressShutter        = 0x00000004
	shutterOff             = 0x00000000
	shutterCompletelyNonAF = 0x00010003
)

// Error codes (EDSDKErrors.h).
const (
	errOK                 = 0x00000000
	errUnimplemented      = 0x00000001
	errNotSupported       = 0x00000007
	errDeviceNotFound     = 0x00000080
	errDeviceBusy         = 0x00000081
	errDeviceInvalid      = 0x00000082
	errDeviceMemoryFull   = 0x00000084
	errDeviceNoDisk       = 0x00000087
	errCommPortInUse      = 0x000000C0
	errCommDisconnected   = 0x000000C1
	errCommDeviceIncompat = 0x000000C2
	errCommUSBBus         = 0x000000C4
	errUSBLockError       = 0x000000D0
	errObjectNotReady     = 0x0000A102
	errTakePictureAFNG    = 0x00008D01
	errTakePictureNoCard  = 0x00008D06
	errTakePictureCardNG  = 0x00008D07
	errTakePictureProtect = 0x00008D08
)

// sdkError is a raw EDSDK status code.
type sdkError struct {
	Op   string
	Code uint32
}

func (e *sdkError) Error() string { return fmt.Sprintf("%s: EDSDK error 0x%08X", e.Op, e.Code) }

func isDisconnect(code uint32) bool {
	switch code {
	case errDeviceNotFound, errDeviceInvalid, errCommDisconnected, errCommUSBBus:
		return true
	}
	return false
}

// DeviceInfo mirrors EdsDeviceInfo (520 bytes).
type DeviceInfoRaw struct {
	PortName          [256]byte
	DeviceDescription [256]byte
	DeviceSubType     uint32
	Reserved          uint32
}

// DirItemInfo mirrors EdsDirectoryItemInfo.
type DirItemInfo struct {
	Size     uint64
	IsFolder int32
	GroupID  uint32
	Option   uint32
	FileName [256]byte
	Format   uint32
	DateTime uint32
}

// Capacity mirrors EdsCapacity; announcing "lots of free space" makes the
// camera send images to the host instead of waiting for card space.
type Capacity struct {
	NumberOfFreeClusters int32
	BytesPerSector       int32
	Reset                int32
}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
