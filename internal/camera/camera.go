// Package camera defines the portable camera abstraction used by the capture
// engine. Platform specific drivers (Canon EDSDK, UVC/FFmpeg, mock) live in
// sub-packages and implement Driver/Camera; nothing in this package imports
// platform code.
package camera

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Support is a three-state capability value. "Unverified" exists because an
// SDK may expose an API that has never been exercised against real hardware;
// the UI must not present such a capability as working.
type Support string

const (
	Supported   Support = "supported"
	Unsupported Support = "unsupported"
	Unverified  Support = "unverified"
)

// Capabilities describes what a connected camera/driver can do at runtime.
type Capabilities struct {
	LiveView         Support  `json:"liveView"`
	StillCapture     Support  `json:"stillCapture"`
	OriginalTransfer Support  `json:"originalTransfer"` // full-resolution original delivered to the PC
	MovieRecord      Support  `json:"movieRecord"`      // camera-native movie start/stop
	MovieTransfer    Support  `json:"movieTransfer"`    // movie file transfer to the PC
	StreamedVideo    Support  `json:"streamedVideo"`    // PC records the frames streamed by the device
	Notes            []string `json:"notes,omitempty"`
}

// Mode is one capture format of a device (resolution, frame rate, pixel format / codec).
type Mode struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	FPSMin float64 `json:"fpsMin"`
	FPSMax float64 `json:"fpsMax"` // 0 = unknown
	Format string  `json:"format"` // mjpeg, yuyv422, nv12 ...
}

// Device kinds shown in the Camera Sources screen.
const (
	KindDSLR        = "dslr"         // Canon body through EDSDK
	KindWebcam      = "webcam"       // built-in or external USB webcam
	KindCaptureCard = "capture-card" // HDMI/USB capture device
	KindVirtual     = "virtual"      // OBS/NDI/EOS Webcam Utility and similar software cameras
	KindSimulated   = "simulated"
	KindCanonUVC    = "dslr-uvc" // a Canon body in USB streaming (UVC) mode
)

// DeviceInfo identifies a discoverable camera source.
type DeviceInfo struct {
	// ID is stable across refreshes and unplug/replug (derived from the USB
	// instance path when the OS provides one, never from a list position).
	ID        string `json:"id"`
	Name      string `json:"name"`
	Driver    string `json:"driver"` // backend: "canon-edsdk", "uvc", "mock"
	Port      string `json:"port,omitempty"`
	Simulated bool   `json:"simulated"` // true only for the mock driver
	Kind      string `json:"kind"`
	Vendor    string `json:"vendor,omitempty"`
	// Interface names how the device is attached and which stack drives it, e.g. "USB (Canon EDSDK)".
	Interface string `json:"interface,omitempty"`
	USBID     string `json:"usbId,omitempty"` // "vid:pid" lower-case hex, when known
	Modes     []Mode `json:"modes,omitempty"` // supported resolutions / frame rates (UVC)
	// DuplicateOf is set when this entry is the same physical device as
	// another listed entry reached through a different backend; it must not
	// be opened as well.
	DuplicateOf string `json:"duplicateOf,omitempty"`
	Note        string `json:"note,omitempty"`
}

// ModeReporter is implemented by cameras that stream in a selectable mode.
type ModeReporter interface {
	ActiveMode() (Mode, bool)
}

// State is the connection state of a camera.
type State string

const (
	StateDisconnected State = "disconnected"
	StateConnected    State = "connected"
	StateLiveView     State = "liveview"
	StateBusy         State = "busy"
	StateError        State = "error"
)

// Frame is one JPEG live-view frame.
type Frame struct {
	Seq  uint64
	JPEG []byte
}

// Photo is a captured still. Data is the camera original (JPEG) exactly as
// delivered by the device.
type Photo struct {
	Data      []byte
	Filename  string // suggested name from the device, may be empty
	Simulated bool   // never true for a real device
	Source    string // human readable, e.g. "Canon EDSDK original" / "UVC frame grab"
}

// EventKind enumerates asynchronous camera notifications.
type EventKind string

const (
	EventDisconnected EventKind = "disconnected"
	EventError        EventKind = "error"
)

// Event is delivered on Camera.Events.
type Event struct {
	Kind EventKind
	Err  error
}

// Camera is a single opened device. All methods must be safe for concurrent
// use and must never block the caller beyond ctx.
type Camera interface {
	Info() DeviceInfo
	Capabilities() Capabilities
	State() State
	Connect(ctx context.Context) error
	// Disconnect releases every device resource. It must be idempotent.
	Disconnect() error
	StartLiveView(ctx context.Context) error
	StopLiveView() error
	// WaitFrame blocks until a frame newer than afterSeq is available.
	WaitFrame(ctx context.Context, afterSeq uint64) (Frame, error)
	// Capture takes a full resolution still and returns the original bytes.
	Capture(ctx context.Context) (Photo, error)
	// Events delivers disconnect/error notifications. The channel is closed
	// after Disconnect.
	Events() <-chan Event
}

// Driver discovers and opens cameras of one kind.
type Driver interface {
	Name() string
	// Available reports whether the driver can run on this machine (SDK DLL
	// present, FFmpeg found, ...). A non-nil error is shown to the operator.
	Available() error
	Discover(ctx context.Context) ([]DeviceInfo, error)
	Open(info DeviceInfo) (Camera, error)
}

// Error codes returned by drivers inside *Error.
const (
	CodeSDKMissing      = "sdk_missing"
	CodeNoDevice        = "no_device"
	CodeDeviceBusy      = "device_busy"
	CodeDisconnected    = "disconnected"
	CodeNotConnected    = "not_connected"
	CodeUnsupported     = "unsupported"
	CodeLiveViewOff     = "liveview_off"
	CodeCaptureFailed   = "capture_failed"
	CodeTimeout         = "timeout"
	CodePlatform        = "platform_unsupported"
	CodeInvalidArgument = "invalid_argument"
	CodeDuplicate       = "duplicate_device"
	CodeBusyOperation   = "operation_in_progress"
)

// Error is an actionable camera error: Message says what happened, Action
// tells the operator what to do about it.
type Error struct {
	Code    string
	Message string
	Action  string
	Cause   error
}

func (e *Error) Error() string {
	s := e.Message
	if e.Cause != nil {
		s += ": " + e.Cause.Error()
	}
	return s
}
func (e *Error) Unwrap() error { return e.Cause }

// Errf builds an *Error.
func Errf(code, action string, cause error, format string, a ...any) *Error {
	return &Error{Code: code, Action: action, Cause: cause, Message: fmt.Sprintf(format, a...)}
}

// CodeOf extracts the camera error code from err ("" if none).
func CodeOf(err error) string {
	var ce *Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// ActionOf extracts the suggested operator action.
func ActionOf(err error) string {
	var ce *Error
	if errors.As(err, &ce) {
		return ce.Action
	}
	return ""
}

// FrameBuffer is a reusable latest-frame holder for drivers.
type FrameBuffer struct {
	mu     sync.Mutex
	cond   chan struct{}
	latest Frame
}

// NewFrameBuffer creates an empty buffer.
func NewFrameBuffer() *FrameBuffer { return &FrameBuffer{cond: make(chan struct{})} }

// Publish stores jpeg (the slice is retained) as the newest frame.
func (b *FrameBuffer) Publish(jpeg []byte) {
	b.mu.Lock()
	b.latest = Frame{Seq: b.latest.Seq + 1, JPEG: jpeg}
	old := b.cond
	b.cond = make(chan struct{})
	b.mu.Unlock()
	close(old)
}

// Latest returns the newest frame and whether one exists.
func (b *FrameBuffer) Latest() (Frame, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.latest, b.latest.Seq > 0
}

// Reset discards the current frame (sequence keeps increasing so waiters
// never see stale data after a restart).
func (b *FrameBuffer) Reset() {
	b.mu.Lock()
	b.latest.JPEG = nil
	b.mu.Unlock()
}

// Wait blocks until a frame with Seq > after exists or ctx ends.
func (b *FrameBuffer) Wait(ctx context.Context, after uint64) (Frame, error) {
	for {
		b.mu.Lock()
		f, ch := b.latest, b.cond
		b.mu.Unlock()
		if f.Seq > after && f.JPEG != nil {
			return f, nil
		}
		select {
		case <-ctx.Done():
			return Frame{}, ctx.Err()
		case <-ch:
		}
	}
}
