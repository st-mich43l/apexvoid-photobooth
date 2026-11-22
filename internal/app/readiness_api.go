package app

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
)

// Booth readiness: can a guest safely start a photo session right now, and if
// not, what must the operator do? Required checks gate guest sessions; optional
// ones (a printer, spare disk space) only inform.
const (
	ReadyStatusReady        = "ready"          // Ready for guests
	ReadyStatusSetup        = "setup_required" // Setup required
	ReadyStatusReconnecting = "reconnecting"   // Reconnecting
	ReadyStatusAttention    = "attention"      // Operator attention required

	ItemOK      = "ok"
	ItemProblem = "problem" // blocks guests when Required
	ItemWarn    = "warn"    // worth knowing, never blocks
	ItemInfo    = "info"

	minFreeBytes  = 500 << 20 // below this a session could fail to save: blocks guests
	warnFreeBytes = 2 << 30   // below this the operator is told
)

// ReadinessItem is one prerequisite.
type ReadinessItem struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	State    string `json:"state"`
	Detail   string `json:"detail,omitempty"`
	Action   string `json:"action,omitempty"`
}

// BoothReadiness is the answer shown in Studio and used by Booth Mode.
type BoothReadiness struct {
	Status   string          `json:"status"`
	CanStart bool            `json:"canStart"`
	Headline string          `json:"headline"`
	Detail   string          `json:"detail,omitempty"` // what the operator should do first
	Guest    string          `json:"guest"`            // a calm sentence suitable for the guest screen
	Items    []ReadinessItem `json:"items"`
	At       string          `json:"at"`
}

type probeCache struct {
	fn   func(root string) (bool, string) // replaceable in tests
	mu   sync.Mutex
	at   time.Time
	ok   bool
	err  string
	path string
}

// writable reports whether a file can be created in the library (cached for a few seconds).
func (p *probeCache) writable(root string) (bool, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.path == root && time.Since(p.at) < 3*time.Second {
		return p.ok, p.err
	}
	p.path, p.at, p.ok, p.err = root, time.Now(), true, ""
	if p.fn != nil {
		p.ok, p.err = p.fn(root)
		return p.ok, p.err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		p.ok, p.err = false, err.Error()
		return p.ok, p.err
	}
	f, err := os.CreateTemp(root, ".ready-*")
	if err != nil {
		p.ok, p.err = false, err.Error()
		return p.ok, p.err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return p.ok, p.err
}

type printerCache struct {
	mu  sync.Mutex
	at  time.Time
	ps  []printing.Printer
	err error
}

func (s *Service) printersCached() ([]printing.Printer, error) {
	c := &s.prnCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < 8*time.Second {
		return c.ps, c.err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c.ps, c.err = s.prn.Driver().List(ctx)
	c.at = time.Now()
	return c.ps, c.err
}

// BoothReadiness evaluates every prerequisite. It changes nothing.
func (s *Service) BoothReadiness() BoothReadiness { return s.readiness(false) }

// readiness evaluates the checks; forStart limits it to what a session needs at
// the moment it begins (no printer, no frame list, the session itself is the one starting).
func (s *Service) readiness(forStart bool) BoothReadiness {
	r := BoothReadiness{Items: []ReadinessItem{}, At: time.Now().Format(time.RFC3339)}
	add := func(key, label string, required bool, state, detail, action string) {
		r.Items = append(r.Items, ReadinessItem{Key: key, Label: label, Required: required, State: state, Detail: detail, Action: action})
	}
	cfg := s.settings.Get()
	cs := s.mgr.Status()
	reconnecting := false

	// --- the photo camera ---
	pref := s.pref(ModePhoto)
	switch {
	case cs.Reconnecting:
		reconnecting = true
		name := "the camera"
		if cs.Device != nil {
			name = cs.Device.Name
		}
		add("camera", "Photo camera", true, ItemProblem, name+" was disconnected; reconnecting automatically.", "Check the USB cable and that the camera is switched on.")
	case cs.Device == nil:
		if pref.Name != "" {
			add("camera", "Photo camera", true, ItemProblem, fmt.Sprintf("Your preferred photo camera “%s” is not connected.", pref.Name), "Connect it and press Refresh on the Camera page, or choose another camera there.")
		} else {
			add("camera", "Photo camera", true, ItemProblem, "No camera is connected.", "Connect a camera and choose it on the Camera page.")
		}
	case cs.State == camera.StateError:
		add("camera", "Photo camera", true, ItemProblem, cs.Device.Name+" reported an error: "+orDash(cs.LastError, "unknown"), orDash(cs.LastAction, "Disconnect and connect the camera again."))
	case !suitable(*cs.Device, ModePhoto):
		add("camera", "Photo camera", true, ItemProblem, cs.Device.Name+" cannot take photos.", "Choose a photo camera on the Camera page.")
	case !forStart && (pref.ID != "" || pref.Name != "") && !matchesPreferred(pref, *cs.Device):
		add("camera", "Photo camera", true, ItemProblem, fmt.Sprintf("%s is connected, but your preferred photo camera is “%s”.", cs.Device.Name, pref.Name), "Connect the preferred camera, or make this one the Photo camera on the Camera page.")
	default:
		add("camera", "Photo camera", true, ItemOK, cs.Device.Name, "")
	}
	if cs.Device != nil && !cs.Reconnecting && cs.State != camera.StateError && cs.Capabilities != nil {
		switch cs.Capabilities.StillCapture {
		case camera.Unsupported:
			add("capture", "Can take photos", true, ItemProblem, "The camera or its driver reported that photos are not supported.", "Put the camera in a still-photo mode, or choose another camera.")
		case camera.Supported:
			add("capture", "Can take photos", true, ItemOK, "A photograph was taken in this session.", "")
		default:
			add("capture", "Can take photos", true, ItemOK, "Not tried yet in this session (the first photo will confirm it).", "")
		}
	}

	// --- the library ---
	root := s.db.Layout.Root
	if ok, why := s.writeProbe.writable(root); ok {
		add("storage", "Library folder is writable", true, ItemOK, root, "")
	} else {
		add("storage", "Library folder is writable", true, ItemProblem, "Cannot write to "+root+": "+why, "Choose another media folder in Settings, or fix the folder's permissions.")
	}
	if err := s.guardWrite(); err != nil {
		add("library", "Library is ready", true, ItemProblem, err.Error(), orDash(userAction(err), ""))
	}
	free := freeBytes(root)
	switch {
	case free < 0:
		add("space", "Free disk space", false, ItemInfo, "unknown", "")
	case free < minFreeBytes:
		add("space", "Free disk space", true, ItemProblem, humanBytes(free)+" free — too little to save photos safely.", "Free some disk space or choose another media folder.")
	case free < warnFreeBytes:
		add("space", "Free disk space", false, ItemWarn, humanBytes(free)+" free.", "Free at least 2 GB before an event.")
	default:
		add("space", "Free disk space", false, ItemOK, humanBytes(free)+" free.", "")
	}

	if !forStart {
		// --- frames ---
		if n := len(s.ListFrames(false).Frames); n == 0 {
			add("frames", "Frames offered to guests", true, ItemProblem, "No frame is switched on.", "Switch at least one frame on under Frames.")
		} else {
			add("frames", "Frames offered to guests", true, ItemOK, fmt.Sprintf("%d offered", n), "")
		}
		// --- a session already open? ---
		switch st := s.eng.Snapshot().State; st {
		case "COUNTDOWN", "CAPTURING", "REVIEW", "SAVING":
			add("session", "No other session running", true, ItemProblem, "A photo session is already open ("+string(st)+").", "Finish or cancel it.")
		}
		if v := s.VideoState().State; v == "recording" || v == "saving" {
			add("session", "No recording running", true, ItemProblem, "A video is being recorded.", "Stop the recording first.")
		}
		if n := len(s.library.Interrupted); n > 0 {
			add("interrupted", "Earlier sessions", false, ItemWarn, fmt.Sprintf("%d session(s) were interrupted; their originals were recovered.", n), "Check them in the Gallery.")
		}
		// --- printer: optional ---
		if cfg.PrinterName == "" {
			add("printer", "Printer", false, ItemInfo, "No printer selected — guests get digital photos only.", "")
		} else {
			ps, err := s.printersCached()
			var found *printing.Printer
			for i := range ps {
				if ps[i].Name == cfg.PrinterName {
					found = &ps[i]
				}
			}
			switch {
			case err != nil && found == nil:
				add("printer", "Printer", false, ItemWarn, "The printer list could not be read: "+err.Error(), "Open the Printer page. Photos are saved digitally either way.")
			case found == nil:
				add("printer", "Printer", false, ItemWarn, fmt.Sprintf("“%s” is not installed.", cfg.PrinterName), "Choose another printer on the Printer page. Photos are saved digitally either way.")
			case found.Available():
				add("printer", "Printer", false, ItemOK, found.Name+": Windows reports it ready. Whether paper actually comes out is not checked.", "")
			default:
				add("printer", "Printer", false, ItemWarn, fmt.Sprintf("%s is %s.", found.Name, found.Status), "Check the printer. Photos are saved digitally either way.")
			}
		}
	}

	// --- verdict ---
	var hard, soft []ReadinessItem
	for _, it := range r.Items {
		switch {
		case it.Required && it.State == ItemProblem:
			hard = append(hard, it)
		case it.State == ItemWarn:
			soft = append(soft, it)
		}
	}
	r.CanStart = len(hard) == 0
	switch {
	case reconnecting:
		r.Status, r.Headline = ReadyStatusReconnecting, "Reconnecting"
		r.Guest = "We are getting the camera ready again. This only takes a moment."
	case len(hard) > 0 && (hard[0].Key == "session"):
		r.Status, r.Headline = ReadyStatusAttention, "Operator attention required"
		r.Guest = "Please ask the host for a moment."
	case len(hard) > 0:
		r.Status, r.Headline = ReadyStatusSetup, "Setup required"
		r.Guest = "The photo booth is not ready yet. Please ask the host."
	case len(soft) > 0:
		r.Status, r.Headline = ReadyStatusAttention, "Ready — operator attention suggested"
	default:
		r.Status, r.Headline = ReadyStatusReady, "Ready for guests"
	}
	if len(hard) > 0 {
		r.Detail = hard[0].Detail
		if hard[0].Action != "" {
			r.Detail += " " + hard[0].Action
		}
	} else if len(soft) > 0 {
		r.Detail = soft[0].Detail
		if soft[0].Action != "" {
			r.Detail += " " + soft[0].Action
		}
	}
	return r
}

func matchesPreferred(p camera.Preferred, d camera.DeviceInfo) bool {
	_, ok := p.Match([]camera.DeviceInfo{d})
	return ok
}

// startCheck is what a session needs at the moment it begins. Guests can never
// start a session on a disconnected camera or an unwritable library, whatever
// the screen believed a moment ago.
func (s *Service) startCheck() error {
	r := s.readiness(true)
	if r.CanStart {
		return nil
	}
	for _, it := range r.Items {
		if it.Required && it.State == ItemProblem {
			return newUserErr(it.Detail, it.Action)
		}
	}
	return newUserErr("The booth is not ready.", "Open Studio to see what is missing.")
}

// RecoverShooting is the one safe way out of a session: it cancels an unfinished
// one (originals already taken stay in the library), waits for a save in flight,
// and releases live view. It is idempotent and serialised, so a double tap on
// Finish or Home cannot start two overlapping resets.
func (s *Service) RecoverShooting() error {
	if err := s.eng.Recover(20 * time.Second); err != nil {
		return newUserErr(err.Error(), "Wait a moment and try again.")
	}
	return nil
}

func userAction(err error) string {
	if e, ok := err.(*UserError); ok {
		return e.Action
	}
	return ""
}
