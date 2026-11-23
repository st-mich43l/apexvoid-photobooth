//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package storage

import (
	"errors"
	"os"
)

func lockFile(*os.File) error {
	return errors.New("library ownership is unsupported on this operating system")
}
func unlockFile(*os.File) error { return nil }

func isLockContention(error) bool { return false }
