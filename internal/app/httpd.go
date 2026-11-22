package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// mediaServer is a loopback-only HTTP server for the two things the web view
// cannot get through bound calls: the endless MJPEG live preview and large
// media files with Range support (video playback). It listens on 127.0.0.1 on
// a random port and requires a per-run random token, so no other program or
// web page can read photos. It uses no network.
type mediaServer struct {
	s     *Service
	srv   *http.Server
	base  string
	token string
}

func startMediaServer(s *Service) (*mediaServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	tok := make([]byte, 16)
	_, _ = rand.Read(tok)
	m := &mediaServer{s: s, token: hex.EncodeToString(tok), base: "http://" + ln.Addr().String()}
	mux := http.NewServeMux()
	mux.HandleFunc("/preview.mjpg", m.auth(m.preview))
	mux.HandleFunc("/snapshot.jpg", m.auth(m.snapshot))
	mux.HandleFunc("/media/", m.auth(m.file))
	mux.HandleFunc("/frame/", m.auth(m.frame))
	m.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = m.srv.Serve(ln) }()
	return m, nil
}

func (m *mediaServer) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = m.srv.Shutdown(ctx)
	_ = m.srv.Close()
}

func (m *mediaServer) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), []byte(m.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// only the app's own web view may embed these (no cross-site reads)
		w.Header().Set("Cache-Control", "no-store")
		h(w, r)
	}
}

func (m *mediaServer) preview(w http.ResponseWriter, r *http.Request) {
	const boundary = "frame"
	f, err := m.s.mgr.WaitFrame(r.Context(), 0)
	if err != nil {
		http.Error(w, "no live view: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
	fl, _ := w.(http.Flusher)
	seq := uint64(0)
	for {
		if _, err := fmt.Fprintf(w, "--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", boundary, len(f.JPEG)); err != nil {
			return
		}
		if _, err := w.Write(f.JPEG); err != nil {
			return
		}
		if _, err := w.Write([]byte("\r\n")); err != nil {
			return
		}
		if fl != nil {
			fl.Flush()
		}
		seq = f.Seq
		f, err = m.s.mgr.WaitFrame(r.Context(), seq)
		if err != nil {
			return // camera gone or client left; the UI reconnects the <img> on status change
		}
	}
}

func (m *mediaServer) snapshot(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	f, err := m.s.mgr.WaitFrame(ctx, 0)
	if err != nil {
		http.Error(w, "no live view", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(f.JPEG)
}

// file serves /media/<id>/<kind> where kind is original|edited|preview|thumb.
// Files are addressed by database id only, never by path.
func (m *mediaServer) file(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/media/"), "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	p, err := m.s.store.FilePath(parts[0], parts[1])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(p))); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeFile(w, r, p) // handles Range for <video>
}

// frame serves /frame/<id>/<variant>?w=<px>&caption=<text>: a preview of a
// frame rendered with sample photos. Use "-" for no colour variant.
func (m *mediaServer) frame(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/frame/"), "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	variant := parts[1]
	if variant == "-" {
		variant = ""
	}
	b, err := m.s.frameJPEG(parts[0], variant, r.URL.Query().Get("caption"), atoi(r.URL.Query().Get("w"), 300))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(b)
}
