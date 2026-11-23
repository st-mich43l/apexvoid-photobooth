package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrLibraryOwned means another process currently holds the library lock.
var ErrLibraryOwned = errors.New("this media library is already in use by another ApexVoid instance")

// Ownership holds an OS-backed advisory lock until Close or process exit. The
// lock file is deliberately never removed: unlinking it would allow a second
// process to lock a different inode while the first still owns the library.
type Ownership struct{ file *os.File }

func AcquireOwnership(root string) (*Ownership, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(root, ".apexvoid.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockFile(f); err != nil {
		_ = f.Close()
		if isLockContention(err) {
			return nil, fmt.Errorf("%w: %v", ErrLibraryOwned, err)
		}
		return nil, fmt.Errorf("lock library: %w", err)
	}
	return &Ownership{file: f}, nil
}
func (o *Ownership) Close() error {
	if o == nil || o.file == nil {
		return nil
	}
	f := o.file
	o.file = nil
	err := unlockFile(f)
	return errors.Join(err, f.Close())
}
