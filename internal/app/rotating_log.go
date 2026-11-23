package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// rotatingLog bounds current logs to limit bytes each, plus count backups.
// It never exports raw messages; they can still contain local private paths.
type rotatingLog struct {
	mu    sync.Mutex
	dir   string
	f     *os.File
	size  int64
	limit int64
	count int
}

func openRotatingLog(dir string, limit int64, count int) (*rotatingLog, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	r := &rotatingLog{dir: dir, limit: limit, count: count}
	f, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	r.f = f
	r.size = st.Size()
	return r, nil
}
func (r *rotatingLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, os.ErrClosed
	}
	// Preserve the writer contract while bounding even a single large message.
	n := len(p)
	if int64(n) > r.limit {
		p = append(append([]byte(nil), p[:r.limit-1]...), '\n')
	}
	if r.size+int64(len(p)) > r.limit {
		if err := r.f.Close(); err != nil {
			return 0, err
		}
		r.f = nil
		_ = os.Remove(filepath.Join(r.dir, fmt.Sprintf("app.log.%d", r.count)))
		for i := r.count - 1; i >= 1; i-- {
			old := filepath.Join(r.dir, fmt.Sprintf("app.log.%d", i))
			next := filepath.Join(r.dir, fmt.Sprintf("app.log.%d", i+1))
			if err := os.Rename(old, next); err != nil && !os.IsNotExist(err) {
				return 0, err
			}
		}
		if err := os.Rename(filepath.Join(r.dir, "app.log"), filepath.Join(r.dir, "app.log.1")); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		f, err := os.OpenFile(filepath.Join(r.dir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return 0, err
		}
		r.f = f
		r.size = 0
	}
	written, err := r.f.Write(p)
	r.size += int64(written)
	if err != nil {
		return written, err
	}
	return n, nil
}
func (r *rotatingLog) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}
