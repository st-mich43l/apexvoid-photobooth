package edsdk

// ref is an opaque EDSDK object handle (EdsBaseRef).
type ref uintptr

// rawCamera is one entry of the EDSDK camera list.
type rawCamera struct {
	Ref  ref
	Name string
	Port string
}

// api is the thin boundary to EDSDK.dll. Every method is called on the single
// SDK thread. The real implementation is dll_windows.go; tests use a fake.
type api interface {
	Init() error
	Terminate()
	// CameraList returns the connected cameras; the caller releases each Ref.
	CameraList() ([]rawCamera, error)
	OpenSession(c ref) error
	CloseSession(c ref) error
	Release(r ref)
	SendCommand(c ref, cmd, param uint32) error
	SetU32(r ref, prop uint32, v uint32) error
	GetU32(r ref, prop uint32) (uint32, error)
	// AnnounceHostSpace tells the camera the host has plenty of space (EdsSetCapacity).
	AnnounceHostSpace(c ref) error
	// DownloadEvf fetches the current live-view frame as JPEG bytes (a copy).
	DownloadEvf(c ref) ([]byte, error)
	DirItem(item ref) (name string, size uint64, err error)
	// Download reads the whole item into memory and completes the transfer.
	Download(item ref, size uint64) ([]byte, error)
	// SkipItem completes/cancels a transfer without keeping the data.
	SkipItem(item ref)
	// SetHandlers registers callbacks (invoked on the SDK thread during PumpEvents).
	SetHandlers(c ref, onObject func(event uint32, item ref), onState func(event, data uint32)) error
	// PumpEvents runs EdsGetEvent and the Windows message loop once.
	PumpEvents()
}
