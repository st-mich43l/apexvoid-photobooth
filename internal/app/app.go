// Package app is the application service: it wires cameras, the shooting
// engine, the media library, video recording, templates and printing, and
// exposes them as the methods bound to the Wails frontend. It has no Wails
// import, so everything here is testable headless.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/edsdk"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/mock"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/uvc"
	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

// Options configure New.
type Options struct {
	Version    string
	ConfigPath string                   // settings.json location ("" = per-user default)
	Emit       func(name string, v any) // push events to the UI (nil = drop)
	PickFolder func() (string, error)   // native folder dialog (nil = unsupported)
	// Test hooks.
	ExtraDrivers  []camera.Driver
	PrintDriver   printing.Driver
	DisableDriver map[string]bool // drop built-in drivers by name
	Tick          time.Duration   // countdown second length
	SkipRestore   bool            // tests: do not auto-connect the preferred camera
	// ResolveLibrary overrides the library location logic (tests simulate Windows folders).
	ResolveLibrary func(*settings.Manager) settings.Resolution
}

// Service is the bound application object. Its exported methods are the UI API.
type Service struct {
	opts        Options
	log         *slog.Logger
	logFile     *rotatingLog
	settings    *settings.Manager
	db          *storage.DB
	owner       *storage.Ownership
	store       *sessions.Store
	tpl         *templates.Library
	mgr         *camera.Manager
	eng         *capture.Engine
	prn         *printing.Manager
	prnStore    printing.Store
	prnStop     chan struct{}
	prnDone     chan struct{}
	restoreDone chan struct{}
	edsdk       *edsdk.Driver
	httpd       *mediaServer

	mu       sync.Mutex
	frames   frameCache // frame preview renders
	frameMu  sync.Mutex // serialises frame snapshots
	rec      recState
	encoder  string
	library  sessions.Report
	shutdown bool

	libRes          settings.Resolution // where the library came from (documents/legacy/custom/fallback)
	migrating       atomic.Bool         // a library migration is copying files
	restartRequired atomic.Bool         // the library was migrated; writes stop until the app restarts
	restoreMsg      string              // why the preferred camera was not connected at start-up
	pinMu           sync.Mutex
	mock            *mock.Driver // the simulated camera driver (enabled by Settings → simulated camera)
	patchMu         sync.Mutex   // serialises settings read-modify-write
	writeProbe      probeCache   // library writability check (cached for a few seconds)
	prnCache        printerCache // printer list for readiness (cached)
	pinFails        int
	pinLockedUntil  time.Time
}

// New builds the service: opens (or creates) the media library, recovers
// interrupted sessions and starts the local preview server. It needs no
// network, server or external database.
func New(o Options) (*Service, error) {
	cfgPath := o.ConfigPath
	if cfgPath == "" {
		cfgPath = settings.DefaultPath()
	}
	sm, err := settings.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	s := &Service{opts: o, settings: sm}
	res := sm.ResolveLibrary()
	if o.ResolveLibrary != nil {
		res = o.ResolveLibrary(sm)
	}
	layout, owner, res, err := acquireLibrary(res)
	if err != nil {
		return nil, err
	}
	s.libRes = res
	s.owner = owner
	started := false
	defer func() {
		if !started {
			if s.httpd != nil {
				s.httpd.Close()
			}
			if s.prnStop != nil {
				close(s.prnStop)
				<-s.prnDone
			}
			if s.db != nil {
				_ = s.db.Close()
			}
			if s.logFile != nil {
				_ = s.logFile.Close()
			}
			_ = owner.Close()
		}
	}()
	s.initLogger(layout)
	db, err := storage.Open(layout)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	s.db = db
	s.store = sessions.New(db)
	if rep, err := s.store.Verify(false); err != nil {
		return nil, fmt.Errorf("library recovery failed; originals have been retained: %w", err)
	} else {
		s.library = rep
		if len(rep.Interrupted)+len(rep.Recovered)+len(rep.Missing) > 0 {
			s.log.Warn("library recovery", "interrupted", rep.Interrupted, "recovered", rep.Recovered, "missing", rep.Missing)
		}
	}
	s.recoverVideos()
	s.tpl = templates.Load(layout.TemplatesDir())
	s.tpl.Staging().Purge(24 * time.Hour) // images imported in the builder but never saved

	// camera drivers
	s.edsdk = edsdk.NewDriver(func() string { return s.settings.Get().CanonSDKDir })
	uv := uvc.NewDriver(
		func() string { return s.settings.Get().FFmpegPath },
		func(id string) uvc.Prefs {
			w, h, fps := s.cameraPrefs(id)
			return uvc.Prefs{Width: w, Height: h, FPS: fps}
		})
	var drivers []camera.Driver
	add := func(d camera.Driver) {
		if !o.DisableDriver[d.Name()] {
			drivers = append(drivers, d)
		}
	}
	add(s.edsdk)
	add(uv)
	s.mock = mock.NewDriver()
	add(&gated{Driver: s.mock, enabled: func() bool { return s.settings.Get().EnableMock }})
	for _, d := range o.ExtraDrivers {
		drivers = append(drivers, d)
	}
	s.mgr = camera.NewManager(drivers...)
	s.mgr.OnChange(func(st camera.Status) {
		if st.Reconnecting {
			s.log.Warn("camera disconnected")
		} else if st.LastCode != "" {
			s.log.Warn("camera error")
		}
		s.emit("camera:status", st)
	})

	s.eng = capture.New(capture.Options{
		Camera: s.mgr, Store: s.store, Tick: o.Tick, CaptureRetries: 1,
		IsSimulated:        func() bool { st := s.mgr.Status(); return st.Device != nil && st.Device.Simulated },
		Finalize:           s.finalizeStrip,
		BeginSession:       s.beginSession,
		KeepSelectedPhotos: s.store.KeepSelectedPhotos,
		OnChange: func(sn capture.Snapshot) {
			if sn.State == capture.Error {
				s.log.Warn("capture failed")
			}
			s.emit("capture:state", sn)
		},
	})
	pd := o.PrintDriver
	if pd == nil {
		pd = printing.NewOSDriver()
	}
	s.prnStore = printing.NewSQLStore(s.db)
	s.prn = printing.NewManager(pd, s.prnStore)
	s.prnStop = make(chan struct{})
	if js := s.prn.Recover(context.Background()); len(js) > 0 {
		s.log.Warn("print jobs recovered after restart", "count", len(js))
	}
	s.prnDone = make(chan struct{})
	go func() { defer close(s.prnDone); s.pollPrints(s.prnStop) }()

	if s.httpd, err = startMediaServer(s); err != nil {
		return nil, fmt.Errorf("start local preview server: %w", err)
	}
	s.restoreDone = make(chan struct{})
	if !o.SkipRestore {
		go func() { defer close(s.restoreDone); s.RestorePreferred() }()
	} else {
		close(s.restoreDone)
	}
	s.log.Info("started", "version", o.Version, "dataDir", layout.Root, "librarySource", res.Source, "migrationPending", res.MigrationPending, "os", runtime.GOOS+"/"+runtime.GOARCH)
	started = true
	return s, nil
}

// acquireLibrary locks the resolved library before creating directories,
// probing writability, migrating SQLite, or recovering any library state. A
// Documents folder that cannot be locked or initialized falls back only when
// the failure is not evidence that another ApexVoid process owns it.
func acquireLibrary(res settings.Resolution) (storage.Layout, *storage.Ownership, settings.Resolution, error) {
	layout := storage.Layout{Root: res.Path}
	owner, err := storage.AcquireOwnership(layout.Root)
	if err != nil {
		if errors.Is(err, storage.ErrLibraryOwned) {
			return storage.Layout{}, nil, res, fmt.Errorf("%w; close the other instance and try again", err)
		}
		if res.Source != settings.SourceDocuments {
			return storage.Layout{}, nil, res, fmt.Errorf("cannot obtain exclusive library access: %w", err)
		}
		res = settings.FallbackLibrary(res, err)
		layout = storage.Layout{Root: res.Path}
		owner, err = storage.AcquireOwnership(layout.Root)
		if err != nil {
			return storage.Layout{}, nil, res, fmt.Errorf("cannot obtain exclusive library access at fallback %s: %w", layout.Root, err)
		}
	}
	if err = layout.Ensure(); err != nil && res.Source == settings.SourceDocuments {
		_ = owner.Close()
		res = settings.FallbackLibrary(res, err)
		layout = storage.Layout{Root: res.Path}
		owner, err = storage.AcquireOwnership(layout.Root)
		if err == nil {
			err = layout.Ensure()
		}
	}
	if err != nil {
		_ = owner.Close()
		return storage.Layout{}, nil, res, fmt.Errorf("the media library folder %s is not usable: %w", res.Path, err)
	}
	return layout, owner, res, nil
}

func (s *Service) emit(name string, v any) {
	if s.opts.Emit != nil {
		s.opts.Emit(name, v)
	}
}

func (s *Service) initLogger(l storage.Layout) {
	s.log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	_ = os.MkdirAll(l.LogsDir(), 0o755)
	// keep 30 days
	if files, _ := filepath.Glob(filepath.Join(l.LogsDir(), "app-*.log")); len(files) > 0 {
		for _, f := range files {
			if st, err := os.Stat(f); err == nil && time.Since(st.ModTime()) > 30*24*time.Hour {
				_ = os.Remove(f)
			}
		}
	}
	f, err := openRotatingLog(l.LogsDir(), 1<<20, 4)
	if err != nil {
		return
	}
	s.logFile = f
	s.log = slog.New(slog.NewTextHandler(f, nil))
	slog.SetDefault(s.log) // device-open diagnostics from the camera/video packages land in the library log
}

// Shutdown releases everything: shooting engine, recording, the camera (so it
// is not left busy), the Canon SDK, the preview server and the database.
func (s *Service) Shutdown() error {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return nil
	}
	s.shutdown = true
	s.mu.Unlock()
	var errs []error
	close(s.prnStop)
	<-s.prnDone
	<-s.restoreDone
	s.eng.Shutdown()
	s.stopRecordingForShutdown()
	if err := s.mgr.Shutdown(); err != nil {
		errs = append(errs, err)
	}
	_ = s.edsdk.Close()
	s.httpd.Close()
	if err := s.db.Close(); err != nil {
		errs = append(errs, err)
	}
	s.log.Info("shutdown complete")
	if s.logFile != nil {
		_ = s.logFile.Close()
	}
	if err := s.owner.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// gated hides a driver until enabled (the simulated camera is a dev tool).
type gated struct {
	camera.Driver
	enabled func() bool
}

func (g *gated) Available() error {
	if !g.enabled() {
		return errors.New("disabled — enable \"Simulated camera\" in Settings (development only)")
	}
	return g.Driver.Available()
}

// UserError carries an operator-facing message plus the action to take. Its
// text is "message\naction" so the UI can split it.
type UserError struct{ Msg, Action string }

func (e *UserError) Error() string {
	if e.Action == "" {
		return e.Msg
	}
	return e.Msg + "\n" + e.Action
}

func userErr(err error) error {
	if err == nil {
		return nil
	}
	var ue *UserError
	if errors.As(err, &ue) {
		return err
	}
	msg := err.Error()
	// keep messages short: drop the wrapped technical chain after the first colon-separated clause when an action exists
	act := camera.ActionOf(err)
	var ce *camera.Error
	if errors.As(err, &ce) {
		msg = ce.Message
		if ce.Cause != nil && !strings.Contains(msg, ce.Cause.Error()) {
			msg += " (" + ce.Cause.Error() + ")"
		}
	}
	return &UserError{Msg: msg, Action: act}
}

func newUserErr(msg, action string) error { return &UserError{Msg: msg, Action: action} }

func (s *Service) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 45*time.Second)
}

// mockDriver exposes the simulated camera driver (tests unplug it).
func (s *Service) mockDriver() *mock.Driver { return s.mock }
