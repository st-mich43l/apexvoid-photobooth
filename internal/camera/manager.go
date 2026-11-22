package camera

import (
	"context"
	"errors"
	"sync"
	"time"
)

// DriverStatus reports whether a driver can run on this machine.
type DriverStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Problem   string `json:"problem,omitempty"`
}

// Status is the UI-facing snapshot of the camera subsystem.
type Status struct {
	Device       *DeviceInfo    `json:"device,omitempty"`
	State        State          `json:"state"`
	Capabilities *Capabilities  `json:"capabilities,omitempty"`
	LiveView     bool           `json:"liveView"`
	Mode         *Mode          `json:"mode,omitempty"` // active stream mode (resolution/fps/format), when known
	Reconnecting bool           `json:"reconnecting"`
	LastError    string         `json:"lastError,omitempty"`
	LastAction   string         `json:"lastAction,omitempty"`
	Drivers      []DriverStatus `json:"drivers"`
}

// Manager owns the single active camera, serialises access to it and
// reconnects after a USB disconnect. It is the only thing the capture engine
// and the UI talk to.
type Manager struct {
	mu        sync.Mutex
	drivers   []Driver
	cam       Camera
	info      DeviceInfo
	liveWant  bool
	lastErr   error
	reconnect bool
	cancelRec context.CancelFunc
	onChange  func(Status)
	shooting  sync.Mutex // held for the whole duration of a capture
	seen      []DeviceInfo // the most recent discovery, to know whether the camera had look-alikes

	// ReconnectInterval is how often a lost camera is looked for again.
	ReconnectInterval time.Duration
}

// NewManager creates a manager over drivers (order = discovery order).
func NewManager(drivers ...Driver) *Manager {
	return &Manager{drivers: drivers, ReconnectInterval: 2 * time.Second}
}

// OnChange registers a status listener (called without locks held).
func (m *Manager) OnChange(f func(Status)) {
	m.mu.Lock()
	m.onChange = f
	m.mu.Unlock()
}

func (m *Manager) notify() {
	m.mu.Lock()
	f := m.onChange
	m.mu.Unlock()
	if f != nil {
		f(m.Status())
	}
}

// Drivers reports driver availability.
func (m *Manager) Drivers() []DriverStatus {
	out := make([]DriverStatus, 0, len(m.drivers))
	for _, d := range m.drivers {
		s := DriverStatus{Name: d.Name(), Available: true}
		if err := d.Available(); err != nil {
			s.Available, s.Problem = false, err.Error()
		}
		out = append(out, s)
	}
	return out
}

// Discover lists devices from every available driver. A driver that is
// unavailable (e.g. Canon SDK DLL missing) is skipped; its reason is in
// Drivers().
func (m *Manager) Discover(ctx context.Context) ([]DeviceInfo, error) {
	var all []DeviceInfo
	var firstErr error
	for _, d := range m.drivers {
		if d.Available() != nil {
			continue
		}
		devs, err := d.Discover(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, devs...)
	}
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}
	MarkDuplicates(all)
	m.mu.Lock()
	m.seen = append([]DeviceInfo(nil), all...)
	m.mu.Unlock()
	return all, nil
}

// lookAlikes counts the listed devices of the same backend and name as info
// (including itself) in the most recent discovery.
func (m *Manager) lookAlikes(info DeviceInfo) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, d := range m.seen {
		if d.Driver == info.Driver && d.Name == info.Name {
			n++
		}
	}
	return n
}

// MarkDuplicates flags entries that are the same physical camera reached
// through more than one backend, so the UI and Connect never open it twice:
//  1. the same USB id reached through two different backends → the later one duplicates the first
//     (the same id twice inside ONE backend is not a duplicate: a Windows Hello module exposes an RGB
//     and an infrared stream, and a capture card may expose several inputs);
//  2. exactly one Canon body via EDSDK and exactly one Canon-vendor UVC device
//     → the UVC entry duplicates the EDSDK one (a body is in one USB mode at a time).
func MarkDuplicates(devs []DeviceInfo) {
	type seen struct{ id, driver string }
	first := map[string]seen{}
	for i := range devs {
		devs[i].DuplicateOf = ""
		if id := devs[i].USBID; id != "" {
			if f, ok := first[id]; ok && f.driver != devs[i].Driver {
				devs[i].DuplicateOf = f.id
				devs[i].Note = "Same physical device as another listed source; open only one."
			} else if !ok {
				first[id] = seen{devs[i].ID, devs[i].Driver}
			}
		}
	}
	var eds, canonUVC []int
	for i, d := range devs {
		switch {
		case d.Driver == "canon-edsdk":
			eds = append(eds, i)
		case d.Vendor == "Canon" && d.Driver != "mock" && d.Kind != KindVirtual:
			canonUVC = append(canonUVC, i)
		}
	}
	if len(eds) == 1 && len(canonUVC) == 1 && devs[canonUVC[0]].DuplicateOf == "" {
		devs[canonUVC[0]].DuplicateOf = devs[eds[0]].ID
		devs[canonUVC[0]].Note = "This Canon body is already listed through the Canon SDK (photo mode). Use that entry for full-resolution photos, or switch the body to USB streaming mode to use it as a video source."
	}
}

// Preferred identifies a remembered camera.
type Preferred struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	USBID  string `json:"usbId"`
	// ExactOnly disables the name/USB-id fallbacks: only the identical id matches.
	// Set when the camera had look-alikes, so another body is never mistaken for it.
	ExactOnly bool `json:"-"`
}

// Match finds the remembered camera among devs: exact ID first, then the same
// backend + USB id, then the same backend + name when that is unique. Duplicate
// entries never match.
func (p Preferred) Match(devs []DeviceInfo) (DeviceInfo, bool) {
	if p.ID == "" && p.Name == "" {
		return DeviceInfo{}, false
	}
	for _, d := range devs {
		if d.ID == p.ID && d.DuplicateOf == "" {
			return d, true
		}
	}
	if p.ExactOnly {
		return DeviceInfo{}, false
	}
	if p.USBID != "" {
		var same []DeviceInfo
		for _, d := range devs {
			if d.Driver == p.Driver && d.USBID == p.USBID && d.DuplicateOf == "" {
				same = append(same, d)
			}
		}
		if len(same) == 1 { // two identical bodies share a USB id: do not guess
			return same[0], true
		}
	}
	var hit []DeviceInfo
	for _, d := range devs {
		if d.Driver == p.Driver && d.Name == p.Name && d.DuplicateOf == "" {
			hit = append(hit, d)
		}
	}
	if len(hit) == 1 {
		return hit[0], true
	}
	return DeviceInfo{}, false
}

func (m *Manager) driverFor(name string) Driver {
	for _, d := range m.drivers {
		if d.Name() == name {
			return d
		}
	}
	return nil
}

// Connect opens and connects the given device, disconnecting any current one.
func (m *Manager) Connect(ctx context.Context, info DeviceInfo) error {
	if info.DuplicateOf != "" {
		return Errf(CodeDuplicate, "Use the other entry for this camera ("+info.DuplicateOf+").", nil, "%s is the same physical camera as another listed source", info.Name)
	}
	// never swap the device under a capture in flight
	if !m.shooting.TryLock() {
		return Errf(CodeBusyOperation, "Wait for the current capture to finish, then switch cameras.", nil, "a capture is in progress")
	}
	defer m.shooting.Unlock()
	m.stopReconnect()
	if err := m.disconnectLocked(false); err != nil {
		_ = err // best effort: the old camera may already be gone
	}
	if err := m.open(ctx, info); err != nil {
		m.setErr(err)
		m.notify()
		return err
	}
	m.mu.Lock()
	m.lastErr, m.liveWant = nil, false
	m.mu.Unlock()
	m.notify()
	return nil
}

func (m *Manager) open(ctx context.Context, info DeviceInfo) error {
	d := m.driverFor(info.Driver)
	if d == nil {
		return Errf(CodeInvalidArgument, "Choose a device from the list.", nil, "unknown camera driver %q", info.Driver)
	}
	if err := d.Available(); err != nil {
		return err
	}
	cam, err := d.Open(info)
	if err != nil {
		return err
	}
	if err := cam.Connect(ctx); err != nil {
		_ = cam.Disconnect()
		return err
	}
	m.mu.Lock()
	m.cam, m.info = cam, info
	m.mu.Unlock()
	go m.watch(cam)
	return nil
}

// watch consumes camera events until the camera's channel closes.
func (m *Manager) watch(cam Camera) {
	for ev := range cam.Events() {
		m.mu.Lock()
		current := m.cam == cam
		m.mu.Unlock()
		if !current {
			return
		}
		switch ev.Kind {
		case EventDisconnected:
			m.handleLost(cam, ev.Err)
			return
		case EventError:
			m.setErr(ev.Err)
			m.notify()
		}
	}
}

func (m *Manager) handleLost(cam Camera, cause error) {
	m.mu.Lock()
	if m.cam != cam {
		m.mu.Unlock()
		return
	}
	info := m.info
	wasLive := m.liveWant
	m.cam = nil
	m.reconnect = true
	m.lastErr = Errf(CodeDisconnected, "Check the USB cable and that the camera is on. The app reconnects automatically.", cause, "camera %s disconnected", info.Name)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelRec = cancel
	m.mu.Unlock()
	exact := m.lookAlikes(info) > 1
	_ = cam.Disconnect() // release handles even though the device is gone
	m.notify()
	go m.reconnectLoop(ctx, info, wasLive, exact)
}

func (m *Manager) reconnectLoop(ctx context.Context, info DeviceInfo, restoreLive, exactOnly bool) {
	t := time.NewTicker(m.ReconnectInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if ctx.Err() != nil { // select picks randomly when both are ready
			return
		}
		d := m.driverFor(info.Driver)
		if d == nil || d.Available() != nil {
			continue
		}
		devs, err := d.Discover(ctx)
		if err != nil {
			continue
		}
		// Only the camera that was lost may come back: exact id, else the same
		// model on the same USB id / a unique name. Two look-alikes never match,
		// so an unrelated camera is never substituted.
		MarkDuplicates(devs)
		match, ok := Preferred{ID: info.ID, Name: info.Name, Driver: info.Driver, USBID: info.USBID, ExactOnly: exactOnly}.Match(devs)
		if !ok {
			continue
		}
		found := &match
		if err := m.open(ctx, *found); err != nil {
			m.setErr(err)
			m.notify()
			continue
		}
		m.mu.Lock()
		if ctx.Err() != nil { // operator disconnected while we were opening
			cam := m.cam
			m.cam = nil
			m.mu.Unlock()
			if cam != nil {
				_ = cam.Disconnect()
			}
			return
		}
		m.reconnect, m.lastErr, m.cancelRec = false, nil, nil
		m.mu.Unlock()
		if restoreLive {
			_ = m.StartLiveView(ctx)
		}
		m.notify()
		return
	}
}

func (m *Manager) stopReconnect() {
	m.mu.Lock()
	if m.cancelRec != nil {
		m.cancelRec()
		m.cancelRec = nil
	}
	m.reconnect = false
	m.mu.Unlock()
}

func (m *Manager) disconnectLocked(notify bool) error {
	m.mu.Lock()
	cam := m.cam
	m.cam = nil
	m.liveWant = false
	m.mu.Unlock()
	var err error
	if cam != nil {
		_ = cam.StopLiveView()
		err = cam.Disconnect()
	}
	if notify {
		m.notify()
	}
	return err
}

// Disconnect releases the camera and stops reconnecting.
func (m *Manager) Disconnect() error {
	m.stopReconnect()
	m.mu.Lock()
	m.lastErr = nil // an operator disconnect is not an error state
	m.mu.Unlock()
	return m.disconnectLocked(true)
}

// Shutdown must be called on application exit: it makes sure the camera is
// released (live view off, session closed) so it is not left busy.
func (m *Manager) Shutdown() error {
	m.stopReconnect()
	m.shooting.Lock() // wait for an in-flight capture to finish
	defer m.shooting.Unlock()
	return m.disconnectLocked(false)
}

func (m *Manager) current() (Camera, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cam == nil {
		msg := "No camera is connected"
		act := "Connect a camera from the Camera page."
		if m.reconnect {
			msg, act = "Camera is disconnected", "Reconnecting automatically; check the USB cable."
		}
		return nil, Errf(CodeNotConnected, act, nil, "%s", msg)
	}
	return m.cam, nil
}

// StartLiveView starts the preview stream.
func (m *Manager) StartLiveView(ctx context.Context) error {
	cam, err := m.current()
	if err != nil {
		return err
	}
	if err := cam.StartLiveView(ctx); err != nil {
		m.setErr(err)
		m.notify()
		return err
	}
	m.mu.Lock()
	m.liveWant, m.lastErr = true, nil
	m.mu.Unlock()
	m.notify()
	return nil
}

// StopLiveView stops the preview stream.
func (m *Manager) StopLiveView() error {
	cam, err := m.current()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.liveWant = false
	m.mu.Unlock()
	err = cam.StopLiveView()
	m.notify()
	return err
}

// WaitFrame returns the next live frame.
func (m *Manager) WaitFrame(ctx context.Context, after uint64) (Frame, error) {
	cam, err := m.current()
	if err != nil {
		return Frame{}, err
	}
	return cam.WaitFrame(ctx, after)
}

// ErrCaptureInProgress is returned when a second capture is attempted.
var ErrCaptureInProgress = Errf(CodeDeviceBusy, "Wait for the current capture to finish.", nil, "a capture is already in progress")

// Capture takes one still. Only one capture may run at a time; the lock is
// always released, including on cancellation and disconnect.
func (m *Manager) Capture(ctx context.Context) (Photo, error) {
	if !m.shooting.TryLock() {
		return Photo{}, ErrCaptureInProgress
	}
	defer m.shooting.Unlock()
	cam, err := m.current()
	if err != nil {
		return Photo{}, err
	}
	p, err := cam.Capture(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		m.setErr(err)
		m.notify()
	}
	return p, err
}

func (m *Manager) setErr(err error) {
	m.mu.Lock()
	m.lastErr = err
	m.mu.Unlock()
}

// Status returns the current snapshot.
func (m *Manager) Status() Status {
	m.mu.Lock()
	cam, info, rec, lerr, live := m.cam, m.info, m.reconnect, m.lastErr, m.liveWant
	m.mu.Unlock()
	st := Status{State: StateDisconnected, Reconnecting: rec, Drivers: m.Drivers()}
	if cam != nil {
		c := cam.Capabilities()
		i := cam.Info()
		st.Device, st.Capabilities, st.State, st.LiveView = &i, &c, cam.State(), live
		if mr, ok := cam.(ModeReporter); ok {
			if md, ok := mr.ActiveMode(); ok {
				st.Mode = &md
			}
		}
	} else if rec {
		i := info
		st.Device = &i
	}
	if lerr != nil {
		st.LastError = lerr.Error()
		st.LastAction = ActionOf(lerr)
		if cam == nil && !rec {
			st.State = StateError
		}
	}
	return st
}

// Camera returns the currently connected camera (nil if none). Used to reach
// optional interfaces such as video.Recordable.
func (m *Manager) Camera() Camera {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cam
}
