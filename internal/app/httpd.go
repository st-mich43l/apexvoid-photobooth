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
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// mediaServer is a loopback-only HTTP server for the two things the web view
// cannot get through bound calls: the endless MJPEG live preview and large
// media files with Range support (video playback). It listens on 127.0.0.1 on
// a random port and requires a per-run random token, so no other program or
// web page can read photos. It uses no network.
type mediaServer struct {
	s       *Service
	srv     *http.Server
	base    string
	token   string
	streams atomic.Int64
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
		started := time.Now()
		sw := &mediaResponseWriter{ResponseWriter: w, status: http.StatusOK}
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), []byte(m.token)) != 1 {
			sw.Header().Set("Cache-Control", "no-store")
			http.Error(sw, "forbidden", http.StatusForbidden)
			m.logRequest(r, sw.status, started, "auth")
			return
		}
		// only the app's own web view may embed these (no cross-site reads)
		sw.Header().Set("Cache-Control", "no-store")
		h(sw, r)
		m.logRequest(r, sw.status, started, "")
	}
}

type mediaResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *mediaResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *mediaResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *mediaResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (m *mediaServer) logRequest(r *http.Request, status int, started time.Time, failure string) {
	if !strings.HasPrefix(r.URL.Path, "/media/") && r.URL.Path != "/preview.mjpg" {
		return
	}
	attrs := []any{"media_type", "preview", "http_status", status, "duration_ms", time.Since(started).Milliseconds()}
	if strings.HasPrefix(r.URL.Path, "/media/") {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/media/"), "/"), "/")
		if len(parts) == 2 {
			attrs[1] = parts[1]
			attrs = append(attrs, "media_id", parts[0])
		}
	}
	if failure != "" {
		attrs = append(attrs, "failure", failure)
	}
	m.s.log.Info("media http request", attrs...)
}

func (m *mediaServer) preview(w http.ResponseWriter, r *http.Request) {
	const boundary = "frame"
	active := m.streams.Add(1)
	started := time.Now()
	defer func() {
		m.streams.Add(-1)
		m.s.log.Info("mjpeg stream closed", "active", m.streams.Load(), "duration_ms", time.Since(started).Milliseconds())
	}()
	m.s.log.Info("mjpeg stream opened", "active", active)
	f, err := m.s.mgr.WaitFrame(r.Context(), 0)
	if err != nil {
		http.Error(w, "no live view: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
	fl, _ := w.(http.Flusher)
	seq := uint64(0)
	for {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && err != http.ErrNotSupported {
			return
		}
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
	kind := parts[1]
	p, err := m.s.store.FilePath(parts[0], kind)
	var info os.FileInfo
	if err == nil {
		info, err = os.Stat(p)
	}
	if (err != nil || info.Size() == 0) && (kind == "preview" || kind == "thumb") {
		fallbackKinds := []string{"preview", "original"}
		if kind == "preview" {
			fallbackKinds = []string{"original"}
		}
		for _, fallback := range fallbackKinds {
			fallbackPath, pathErr := m.s.store.FilePath(parts[0], fallback)
			if pathErr != nil {
				continue
			}
			fallbackInfo, statErr := os.Stat(fallbackPath)
			if statErr == nil && fallbackInfo.Size() > 0 {
				m.s.log.Warn("media derivative fallback", "media_id", parts[0], "media_type", kind, "fallback_type", fallback)
				p, info, err = fallbackPath, fallbackInfo, nil
				break
			}
		}
	}
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		m.s.log.Warn("media file unavailable", "media_id", parts[0], "media_type", parts[1], "reason", errorClass(err))
		http.NotFound(w, r)
		return
	}
	if info.Size() == 0 {
		w.Header().Set("Cache-Control", "no-store")
		m.s.log.Warn("media file empty", "media_id", parts[0], "media_type", parts[1])
		http.Error(w, "empty media", http.StatusInternalServerError)
		return
	}
	m.s.log.Debug("media file available", "media_id", parts[0], "media_type", parts[1], "size_bytes", info.Size())
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(p))); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeFile(w, r, p) // handles Range for <video>
}

func errorClass(err error) string {
	switch {
	case os.IsNotExist(err):
		return "not_found"
	case os.IsPermission(err):
		return "permission_denied"
	default:
		return "io_or_database_error"
	}
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
