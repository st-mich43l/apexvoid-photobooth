package app

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/edsdk"
)

// Readiness states. "ok" means the thing was actually observed working in this
// run; "not_tried" means nothing has exercised it yet. Nothing is ever shown as
// working on the strength of a fake SDK or a successful build.
const (
	ReadyOK       = "ok"
	ReadyProblem  = "problem"
	ReadyNotTried = "not_tried"
)

// Readiness is one line of the Canon setup / diagnostics panel.
type Readiness struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
	Action string `json:"action,omitempty"`
}

// CanonActive describes the connected camera when it is a Canon body.
type CanonActive struct {
	Name         string              `json:"name"`
	Interface    string              `json:"interface"`
	Port         string              `json:"port,omitempty"`
	USBID        string              `json:"usbId,omitempty"`
	State        camera.State        `json:"state"`
	LiveView     bool                `json:"liveView"`
	Capabilities camera.Capabilities `json:"capabilities"`
	Counters     *edsdk.Diag         `json:"counters,omitempty"`
}

// CanonStatus is everything the Camera page needs to explain the state of the
// Canon integration and what to do next.
type CanonStatus struct {
	Platform       string              `json:"platform"`
	SDK            edsdk.SDKReport     `json:"sdk"`
	Driver         *edsdk.Health       `json:"driver,omitempty"` // nil until the SDK thread has started
	Detected       []camera.DeviceInfo `json:"detected"`
	Conflicts      []string            `json:"conflicts"`
	Active         *CanonActive        `json:"active,omitempty"`
	Items          []Readiness         `json:"items"`
	Recommendation string              `json:"recommendation,omitempty"`
	Guide          []string            `json:"guide"`
	Verification   string              `json:"verification"`
}

// canonGuide is the Windows setup guide for the EOS R50.
var canonGuide = []string{
	"Set the camera's USB connection app to Photo Import / Remote Control (not Webcam or Streaming): Menu → Wrench → Communication settings → USB connection app.",
	"Turn the mode dial to a still-photo mode (not Movie) and fit a lens. Use a charged battery or the AC adapter, and switch off auto power-off while testing.",
	"Set image quality to JPEG or RAW+JPEG. RAW-only cannot be used: no viewable file reaches the PC.",
	"Close EOS Utility, EOS Webcam Utility, Canon Camera Connect, Windows Photos/Camera import and any other program that uses the camera. Only one program can own it.",
	"Use a good USB data cable straight into the PC (no hub) and make sure the camera is switched on.",
	"Copy Canon's 64-bit EDSDK.dll and EdsImage.dll (and the rest of the Dll folder) next to the application, into .\\sdk\\canon, or choose the folder in Settings.",
}

const canonVerification = "Automated tests use a simulated Canon SDK and the Windows build is checked in CI; neither proves that a real EOS R50 works. " +
	"A capability is shown as working only after it succeeded on the connected camera in this session. Real-hardware results are recorded with the hardware validation command (docs/HARDWARE_TESTS.md)."

// CanonStatus inspects the Canon integration without changing anything: it
// reads the SDK folder statically, lists attached bodies and reports what the
// connected camera has actually done in this session.
func (s *Service) CanonStatus() CanonStatus {
	st := CanonStatus{Platform: runtime.GOOS + "/" + runtime.GOARCH, Detected: []camera.DeviceInfo{}, Conflicts: []string{}, Guide: canonGuide, Verification: canonVerification}
	st.SDK = edsdk.SDKStatus(s.settings.Get().CanonSDKDir)
	if h, ok := s.edsdk.Health(); ok {
		st.Driver = &h
	}
	if runtime.GOOS == "windows" {
		st.Conflicts = edsdk.Conflicts()
	}
	add := func(key, label, state, detail, action string) {
		st.Items = append(st.Items, Readiness{Key: key, Label: label, State: state, Detail: detail, Action: action})
	}

	// 1. SDK files
	if st.SDK.OK() {
		add("sdk", "Canon SDK files", ReadyOK, fmt.Sprintf("%s (%s)", st.SDK.Path, st.SDK.Machine), "")
	} else {
		add("sdk", "Canon SDK files", ReadyProblem, st.SDK.Problem, st.SDK.Action)
	}

	// 2. SDK thread
	switch {
	case !st.SDK.OK():
		add("init", "SDK initialised", ReadyNotTried, "Needs the SDK files first.", "")
	case st.Driver == nil:
		add("init", "SDK initialised", ReadyNotTried, "Starts when the camera is first used.", "")
	case st.Driver.Stuck:
		add("init", "SDK initialised", ReadyProblem, fmt.Sprintf("the SDK call “%s” has been running for %d s", st.Driver.Op, st.Driver.RunningMs/1000),
			"Unplug and replug the camera. If it does not recover, close and reopen ApexVoid.")
	default:
		add("init", "SDK initialised", ReadyOK, "The SDK thread is running.", "")
	}

	// 3. programs that own the camera
	if len(st.Conflicts) > 0 {
		add("conflicts", "Other programs using the camera", ReadyProblem, "running: "+strings.Join(st.Conflicts, ", "), "Close them, then connect the camera.")
	} else if runtime.GOOS == "windows" {
		add("conflicts", "Other programs using the camera", ReadyOK, "none running", "")
	}

	// 4. detection
	var derr error
	if st.SDK.OK() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		devs, err := s.edsdk.Discover(ctx)
		cancel()
		derr = err
		if devs != nil {
			st.Detected = devs
		}
	}
	switch {
	case !st.SDK.OK():
		add("detected", "Camera detected", ReadyNotTried, "Needs the SDK files first.", "")
	case derr != nil:
		add("detected", "Camera detected", ReadyProblem, derr.Error(), camera.ActionOf(derr))
	case len(st.Detected) == 0:
		add("detected", "Camera detected", ReadyProblem, "No Canon camera is attached.",
			"Switch the camera on, check the USB cable, and set the USB connection app to Photo Import / Remote Control.")
	default:
		names := make([]string, 0, len(st.Detected))
		for _, d := range st.Detected {
			names = append(names, d.Name)
		}
		add("detected", "Camera detected", ReadyOK, strings.Join(names, ", "), "")
	}

	// 5. the connected camera
	cs := s.mgr.Status()
	if cs.Device != nil && cs.Device.Driver == edsdk.DriverName {
		a := &CanonActive{Name: cs.Device.Name, Interface: cs.Device.Interface, Port: cs.Device.Port, USBID: cs.Device.USBID, State: cs.State, LiveView: cs.LiveView}
		if cs.Capabilities != nil {
			a.Capabilities = *cs.Capabilities
		}
		if c, ok := s.mgr.Camera().(*edsdk.Camera); ok {
			d := c.Diagnostics()
			a.Counters = &d
		}
		st.Active = a
	}
	switch {
	case st.Active != nil && cs.Reconnecting:
		add("connected", "Camera connected", ReadyProblem, "The USB connection dropped; reconnecting automatically.", "Check the USB cable and that the camera is on.")
	case st.Active != nil && (cs.State == camera.StateConnected || cs.State == camera.StateLiveView):
		add("connected", "Camera connected", ReadyOK, st.Active.Name, "")
	case st.Active != nil:
		add("connected", "Camera connected", ReadyProblem, "state: "+string(cs.State), "Disconnect and connect again.")
	default:
		add("connected", "Camera connected", ReadyNotTried, "No Canon camera is connected yet.", "Choose the camera under Available sources and press Connect.")
	}

	// 6-8. capabilities: only what really happened on this camera
	capLine := func(key, label string, sup camera.Support, working, untried string) {
		switch {
		case st.Active == nil:
			add(key, label, ReadyNotTried, "Connect the camera first.", "")
		case sup == camera.Supported:
			add(key, label, ReadyOK, working, "")
		case sup == camera.Unsupported:
			add(key, label, ReadyProblem, "The camera or SDK reported that this is not supported.", "Check the camera mode (still photo, not Movie) and use an EDSDK version that lists this camera.")
		default:
			add(key, label, ReadyNotTried, untried, "")
		}
	}
	var caps camera.Capabilities
	if st.Active != nil {
		caps = st.Active.Capabilities
	}
	capLine("liveview", "Live view", caps.LiveView, "Frames were received in this session.", "Not tried yet: start the preview.")
	capLine("still", "Still capture (shutter)", caps.StillCapture, "A photograph was taken in this session.", "Not tried yet: take a test photo.")
	capLine("original", "Full-resolution original transferred to the PC", caps.OriginalTransfer, "A complete JPEG was received in this session.", "Not tried yet: take a test photo.")

	// 9. the last error, classified
	if cs.LastError != "" {
		add("lasterror", "Last camera error", ReadyProblem, classifyCanonError(cs.LastCode)+": "+cs.LastError, cs.LastAction)
	}

	for _, it := range st.Items {
		if it.State == ReadyProblem && it.Action != "" {
			st.Recommendation = it.Action
			break
		}
	}
	if st.Recommendation == "" {
		for _, it := range st.Items {
			if it.State == ReadyNotTried && it.Action != "" {
				st.Recommendation = it.Action
				break
			}
		}
	}
	return st
}

// classifyCanonError turns a camera error code into the distinction the
// operator needs: what kind of problem it is.
func classifyCanonError(code string) string {
	switch code {
	case camera.CodeSDKMissing:
		return "Canon SDK missing or unusable"
	case camera.CodeDeviceBusy:
		return "Camera busy (owned by another program, or still working)"
	case camera.CodeUnsupported:
		return "Not supported by this camera or SDK version"
	case camera.CodeDisconnected, camera.CodeNotConnected:
		return "Camera disconnected"
	case camera.CodeTimeout:
		return "The camera did not respond in time"
	case camera.CodeCaptureFailed:
		return "Capture failed"
	case camera.CodePlatform:
		return "Not available on this platform"
	}
	return "Camera error"
}
