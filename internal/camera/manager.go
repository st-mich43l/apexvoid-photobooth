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
	return all, nil
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
	_ = cam.Disconnect() // release handles even though the device is gone
	m.notify()
	go m.reconnectLoop(ctx, info, wasLive)
}

func (m *Manager) reconnectLoop(ctx context.Context, info DeviceInfo, restoreLive bool) {
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
		var found *DeviceInfo
		for i := range devs {
			if devs[i].ID == info.ID || (found == nil && devs[i].Name == info.Name) {
				found = &devs[i]
			}
		}
		if found == nil {
			continue
		}
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
