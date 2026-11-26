//go:build uxharness

// Command uxharness runs the REAL application service (simulated camera, real
// Go frame renderer, fake printer) headless behind a tiny HTTP bridge, so the
// guest UI can be driven and screenshotted in an ordinary browser. Development
// and screenshot tool only; it is not part of the product build (build tag).
//
//	go run -tags uxharness ./cmd/uxharness -addr 127.0.0.1:5199 -dist cmd/photobooth/frontend_dist
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/app"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/mock"
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

type fakePrinter struct {
	mu sync.Mutex
	n  int
}

func (f *fakePrinter) Name() string { return "fake" }
func (f *fakePrinter) List(context.Context) ([]printing.Printer, error) {
	return []printing.Printer{{Name: "Lab", Default: true, Status: "ready"}}, nil
}
func (f *fakePrinter) Capabilities(context.Context, string) (printing.Caps, error) {
	return printing.Caps{Papers: []printing.Paper{{ID: 119, Name: "4x6 in", WidthIn: 4, HeightIn: 6}},
		Orientations: []printing.Orientation{printing.Portrait, printing.Landscape}, MaxCopies: 10, Resolutions: [][2]int{{300, 300}}}, nil
}
func (f *fakePrinter) Geometry(_ context.Context, _ string, p printing.Paper, o printing.Orientation) (printing.PageGeometry, error) {
	return printing.GeometryFor(p, o, 300, 0), nil
}
func (f *fakePrinter) Submit(context.Context, printing.SubmitRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	return fmt.Sprint(f.n), nil
}
func (f *fakePrinter) Status(_ context.Context, _ string, nativeID string) (printing.JobState, error) {
	if nativeID == "1" { // the first job "finishes", so screenshots show a completed job with its reprint action
		return printing.JobState{Status: printing.StatusCompleted, Detail: "The spooler reports the job as printed (this is not proof that paper came out)."}, nil
	}
	return printing.JobState{Status: printing.StatusQueued, Detail: "Sent to the Windows print queue."}, nil
}

// fakeWebcam presents the mock camera as a physical USB webcam (backend "uvc", not simulated) so the operator
// screens can be captured in their "real webcam" state. It is a harness stand-in only: nothing here proves anything
// about real webcams, and the screenshots say so.
type fakeWebcam struct{ *mock.Driver }

func (fakeWebcam) Name() string { return "uvc" }
func (fakeWebcam) Discover(context.Context) ([]camera.DeviceInfo, error) {
	return []camera.DeviceInfo{{ID: "uvc:harness-1", Name: "USB Webcam (harness stand-in)", Driver: "uvc", Kind: camera.KindWebcam, USBID: "0000:0000",
		Modes: []camera.Mode{{Width: 1280, Height: 720, FPSMax: 30, Format: "mjpeg"}}}}, nil
}
func (d fakeWebcam) Open(i camera.DeviceInfo) (camera.Camera, error) {
	c, err := d.Driver.Open(i)
	if err != nil {
		return nil, err
	}
	return webcamCam{c, i}, nil
}

type webcamCam struct {
	camera.Camera
	info camera.DeviceInfo
}

func (w webcamCam) Info() camera.DeviceInfo { return w.info }

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func (h *hub) emit(name string, v any) {
	b, _ := json.Marshal(map[string]any{"name": name, "data": v})
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.subs {
		select {
		case c <- b:
		default:
		}
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:5199", "listen address (loopback)")
	dist := flag.String("dist", "cmd/photobooth/frontend_dist", "built frontend")
	tick := flag.Duration("tick", 700*time.Millisecond, "countdown second length")
	shots := flag.Int("shots", 4, "default photos")
	noCamera := flag.Bool("no-camera", false, "no camera at all (shows the 'not ready' states)")
	webcam := flag.Bool("webcam", false, "a fake USB webcam instead of the simulated camera (operator-screen screenshots)")
	fewFrames := flag.Bool("few-frames", false, "leave the default set of frames (instead of switching every design on)")
	printer := flag.Bool("printer", true, "configure the fake printer")
	seed := flag.Int("seed-sessions", 0, "create synthetic completed sessions for gallery screenshots")
	seedGallery := flag.Bool("seed-gallery", false, "create the Gallery session-detail scenarios (compositions, videos, print jobs)")
	flag.Parse()

	dir, _ := os.MkdirTemp("", "uxharness")
	cfg := filepath.Join(dir, "settings.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.EnableMock, st.CountdownSeconds, st.Shots = filepath.Join(dir, "media"), !*noCamera && !*webcam, 3, *shots
	if *printer {
		st.PrinterName, st.PrintPaper = "Lab", "4x6 in"
	}
	if err := sm.Save(st); err != nil {
		log.Fatal(err)
	}
	if *seed > 0 {
		db, err := storage.Open(storage.Layout{Root: st.MediaDir})
		if err != nil {
			log.Fatal(err)
		}
		store := sessions.New(db)
		for i := 0; i < *seed; i++ {
			t := time.Now().UTC().Add(-time.Duration(i) * time.Hour)
			store.SetClock(func() time.Time { return t })
			sess, err := store.Create("photo", 1, "plain.strip4", true)
			if err != nil {
				log.Fatal(err)
			}
			img := image.NewRGBA(image.Rect(0, 0, 240, 180))
			for y := 0; y < 180; y++ {
				for x := 0; x < 240; x++ {
					img.Set(x, y, color.RGBA{R: uint8((i*19 + x) % 256), G: uint8((y + i*9) % 256), B: uint8((x + y) / 2), A: 255})
				}
			}
			var b bytes.Buffer
			if err = jpeg.Encode(&b, img, nil); err != nil {
				log.Fatal(err)
			}
			if _, err = store.AddPhoto(sess.ID, sessions.PhotoInput{Data: b.Bytes(), Simulated: true, Source: "generated screenshot sample"}); err != nil {
				log.Fatal(err)
			}
			if err = store.SetFrame(sess.ID, sessions.FrameRef{ID: "plain.strip4", Name: "Classic strip"}); err != nil {
				log.Fatal(err)
			}
			if err = store.SetStatus(sess.ID, sessions.StatusCompleted, ""); err != nil {
				log.Fatal(err)
			}
		}
		if err = db.Close(); err != nil {
			log.Fatal(err)
		}
	}

	var gallery []seededSession
	if *seedGallery {
		gallery = seedGalleryStore(st.MediaDir)
	}

	h := &hub{subs: map[chan []byte]struct{}{}}
	opts := app.Options{Version: "0.2.0", ConfigPath: cfg, Emit: h.emit, Tick: *tick,
		DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}, PrintDriver: &fakePrinter{}, PickFolder: func() (string, error) { return filepath.Join(dir, "sample-exports"), nil }}
	var wd *fakeWebcam
	if *webcam {
		wd = &fakeWebcam{mock.NewDriver()}
		opts.ExtraDrivers = []camera.Driver{wd}
	}
	svc, err := app.New(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer svc.Shutdown()
	// every built-in design on, so the gallery shows the full catalogue
	if r := svc.ListFrames(true); !*fewFrames {
		for _, f := range r.Frames {
			_ = svc.SetFrameEnabled(f.ID, true)
		}
	}

	if *webcam {
		if err := svc.ConnectCamera("uvc:harness-1"); err != nil {
			log.Printf("connect webcam stand-in: %v", err)
		}
	} else if !*noCamera {
		if err := svc.ConnectCamera("mock-0"); err != nil {
			log.Printf("connect simulated camera: %v", err)
		}
	}

	if *seedGallery {
		finishGallerySeed(svc, gallery)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(*dist)))
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		c := make(chan []byte, 64)
		h.mu.Lock()
		h.subs[c] = struct{}{}
		h.mu.Unlock()
		defer func() { h.mu.Lock(); delete(h.subs, c); h.mu.Unlock() }()
		fmt.Fprint(w, ": hi\n\n")
		fl.Flush()
		for {
			select {
			case b := <-c:
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
	mux.HandleFunc("/dev/camera", func(w http.ResponseWriter, r *http.Request) {
		if wd != nil {
			wd.SetPresent(r.URL.Query().Get("present") == "1")
			return
		}
		app.DevSetCameraPresent(svc, r.URL.Query().Get("present") == "1")
	})
	sv := reflect.ValueOf(svc)
	mux.HandleFunc("/rpc/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/rpc/")
		m := sv.MethodByName(name)
		if !m.IsValid() {
			http.Error(w, "no such method", 404)
			return
		}
		var raw []json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)
		mt := m.Type()
		if mt.NumIn() != len(raw) {
			http.Error(w, "bad arity", 400)
			return
		}
		in := make([]reflect.Value, len(raw))
		for i := range raw {
			p := reflect.New(mt.In(i))
			if err := json.Unmarshal(raw[i], p.Interface()); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			in[i] = p.Elem()
		}
		out := m.Call(in)
		var res any
		for _, o := range out {
			if e, ok := o.Interface().(error); ok && e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
			if o.Type() != reflect.TypeOf((*error)(nil)).Elem() {
				res = o.Interface()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})
	log.Printf("uxharness on http://%s (media in %s)", *addr, dir)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
