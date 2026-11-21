package video

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Single-file build: when the program is built with `-tags embedffmpeg` an
// LGPL FFmpeg (gzip-compressed) is linked into the executable as plain data.
// It is NOT linked as a library: at first use it is unpacked to
//
//	<user cache dir>\ApexVoidPhotobooth\bin\ffmpeg-<hash>.exe
//
// and run as a separate process, exactly like an ffmpeg.exe placed beside the
// app (which still takes precedence, as does the path in Settings). The licence
// text is unpacked next to it. The normal build embeds nothing.

var (
	builtInOnce sync.Once
	builtInPath string
	builtInErr  error
)

// builtInFFmpeg returns the path of the unpacked built-in ffmpeg ("" when this
// build has none).
func builtInFFmpeg() (string, error) {
	gz, lic, ok := embeddedFFmpeg()
	if !ok {
		return "", nil
	}
	builtInOnce.Do(func() {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		builtInPath, builtInErr = extractEmbedded(filepath.Join(base, "ApexVoidPhotobooth", "bin"), gz, lic)
	})
	return builtInPath, builtInErr
}

// IsBuiltIn reports whether path is the unpacked copy from this executable.
func IsBuiltIn(path string) bool {
	if _, _, ok := embeddedFFmpeg(); !ok || path == "" {
		return false
	}
	p, err := builtInFFmpeg()
	return err == nil && p == path
}

// gzipSize reads the uncompressed size from the gzip trailer (ISIZE, mod 2^32).
func gzipSize(gz []byte) int64 {
	if len(gz) < 4 {
		return -1
	}
	return int64(binary.LittleEndian.Uint32(gz[len(gz)-4:]))
}

// extractEmbedded unpacks gz into dir as ffmpeg-<hash>.exe (and the licence as
// FFMPEG-LICENSE.txt) and returns the executable path. It is idempotent: an
// existing file of the right size is reused; a missing or truncated one is
// rebuilt through a temporary file + rename, so a crash or a second process
// never leaves a half-written executable behind.
func extractEmbedded(dir string, gz, license []byte) (string, error) {
	want := gzipSize(gz)
	if want <= 0 {
		return "", errors.New("the built-in ffmpeg data is empty")
	}
	sum := sha256.Sum256(gz)
	exe := filepath.Join(dir, "ffmpeg-"+hex.EncodeToString(sum[:])[:12]+exeSuffix())
	if st, err := os.Stat(exe); err == nil && st.Size() == want {
		writeLicense(dir, license)
		return exe, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return "", fmt.Errorf("the built-in ffmpeg data is corrupt: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".unpack-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }
	n, err := io.Copy(tmp, zr)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("unpacking ffmpeg: %w", err)
	}
	if n != want {
		cleanup()
		return "", fmt.Errorf("unpacked %d bytes, expected %d", n, want)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	_ = os.Chmod(tmpName, 0o755)
	if err := os.Rename(tmpName, exe); err != nil {
		// another process may have finished first (Windows cannot replace a running exe)
		_ = os.Remove(tmpName)
		if st, serr := os.Stat(exe); serr == nil && st.Size() == want {
			writeLicense(dir, license)
			return exe, nil
		}
		return "", err
	}
	writeLicense(dir, license)
	return exe, nil
}

func writeLicense(dir string, license []byte) {
	if len(license) > 0 {
		_ = os.WriteFile(filepath.Join(dir, "FFMPEG-LICENSE.txt"), license, 0o644)
	}
}

func exeSuffix() string {
	if exeName() == "ffmpeg.exe" {
		return ".exe"
	}
	return ""
}
