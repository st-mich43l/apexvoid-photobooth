//go:build uxharness

// Command uxharness runs the REAL application service (simulated camera, real
// Go frame renderer, fake printer) headless behind a tiny HTTP bridge, so the
// guest UI can be driven and screenshotted in an ordinary browser. Development
// and screenshot tool only; it is not part of the product build (build tag).
//
//	go run -tags uxharness ./cmd/uxharness -addr 127.0.0.1:5199 -dist cmd/photobooth/frontend_dist
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/app"
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
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
func (f *fakePrinter) Status(context.Context, string, string) (printing.JobState, error) {
	return printing.JobState{Status: printing.StatusQueued, Detail: "Sent to the Windows print queue."}, nil
}

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
	noCamera := flag.Bool("no-camera", false, "simulated camera off (shows the 'not ready' welcome)")
	printer := flag.Bool("printer", true, "configure the fake printer")
	flag.Parse()

	dir, _ := os.MkdirTemp("", "uxharness")
	cfg := filepath.Join(dir, "settings.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.EnableMock, st.CountdownSeconds, st.Shots = filepath.Join(dir, "media"), !*noCamera, 3, *shots
	if *printer {
		st.PrinterName, st.PrintPaper = "Lab", "4x6 in"
	}
	if err := sm.Save(st); err != nil {
		log.Fatal(err)
	}

	h := &hub{subs: map[chan []byte]struct{}{}}
	svc, err := app.New(app.Options{Version: "uxharness", ConfigPath: cfg, Emit: h.emit, Tick: *tick,
		DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}, PrintDriver: &fakePrinter{}})
	if err != nil {
		log.Fatal(err)
	}
	defer svc.Shutdown()
	// every built-in design on, so the gallery shows the full catalogue
	if r := svc.ListFrames(true); true {
		for _, f := range r.Frames {
			_ = svc.SetFrameEnabled(f.ID, true)
		}
	}

	if !*noCamera {
		if err := svc.ConnectCamera("mock-0"); err != nil {
			log.Printf("connect simulated camera: %v", err)
		}
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
