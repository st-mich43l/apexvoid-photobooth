package edsdk

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var embeddedMu sync.Mutex

// unpackSDK keeps each SDK version in its own directory, so the two DLLs are
// always loaded together and upgrades never overwrite an in-use library.
func unpackSDK(base string, sdk, image []byte) (string, error) {
	if len(sdk) == 0 || len(image) == 0 {
		return "", errors.New("embedded Canon SDK is incomplete")
	}
	embeddedMu.Lock()
	defer embeddedMu.Unlock()
	h := sha256.New()
	_, _ = h.Write(sdk)
	_, _ = h.Write(image)
	dir := filepath.Join(base, hex.EncodeToString(h.Sum(nil)))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{"EDSDK.dll", sdk}, {"EdsImage.dll", image}} {
		path := filepath.Join(dir, f.name)
		expected := sha256.Sum256(f.data)
		if existing, err := os.ReadFile(path); err == nil && sha256.Sum256(existing) == expected {
			continue
		}
		tmp, err := os.CreateTemp(dir, ".unpack-*")
		if err != nil {
			return "", err
		}
		name := tmp.Name()
		if _, err = tmp.Write(f.data); err == nil {
			err = tmp.Sync()
		}
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			// Windows cannot replace an existing file with Rename.
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				err = removeErr
			}
		}
		if err == nil {
			err = os.Rename(name, path)
		}
		_ = os.Remove(name)
		if err != nil {
			return "", fmt.Errorf("unpack %s: %w", f.name, err)
		}
	}
	return dir, nil
}

func bundledSDKDir() (string, error) {
	sdk, image := bundledSDK()
	if len(sdk) == 0 && len(image) == 0 {
		return "", nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find Canon SDK cache: %w", err)
	}
	return unpackSDK(filepath.Join(base, "ApexVoidPhotobooth", "canon-sdk"), sdk, image)
}
