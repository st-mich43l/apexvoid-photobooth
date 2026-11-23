//go:build windows

package edsdk

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

const (
	evAllObject = 0x00000200 // kEdsObjectEvent_All
	evAllState  = 0x00000300 // kEdsStateEvent_All
)

func exeDir() string {
	if p, err := os.Executable(); err == nil {
		return filepath.Dir(p)
	}
	return ""
}

// SDKStatus statically inspects the Canon SDK folder (no DLL is loaded).
func SDKStatus(configured string) SDKReport { return Inspect(configured, exeDir()) }

func sdkAvailable(dir string) error {
	r := Inspect(dir, exeDir())
	if r.OK() {
		return nil
	}
	code := camera.CodeSDKMissing
	if r.Kind == SDKWrongArch || r.Kind == SDKMissingExport {
		code = camera.CodeUnsupported
	}
	return camera.Errf(code, r.Action, errors.New(r.Problem), "Canon EDSDK is not usable: %s", sdkKindText(r.Kind))
}

func sdkKindText(k string) string {
	switch k {
	case SDKNotFound:
		return "not found"
	case SDKWrongArch:
		return "wrong architecture"
	case SDKInvalid:
		return "invalid DLL"
	case SDKMissingDependency:
		return "a dependent DLL is missing"
	case SDKMissingExport:
		return "incompatible version"
	}
	return k
}

// resolveDependency reports whether a DLL the SDK imports can be found: beside
// the SDK, in the Windows system/Windows directories, in the application
// directory or on PATH. API-set names are provided by the OS.
func resolveDependency(name, dir string) bool {
	l := strings.ToLower(name)
	if strings.HasPrefix(l, "api-ms-win-") || strings.HasPrefix(l, "ext-ms-") {
		return true
	}
	var dirs []string
	dirs = append(dirs, dir, exeDir())
	if sys, err := windows.GetSystemDirectory(); err == nil {
		dirs = append(dirs, sys)
	}
	if wd, err := windows.GetWindowsDirectory(); err == nil {
		dirs = append(dirs, wd)
	}
	dirs = append(dirs, filepath.SplitList(os.Getenv("PATH"))...)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, name)); err == nil {
			return true
		}
	}
	return false
}

func loadDLL(dir string) (api, error) {
	p, err := locateSDK(dir, exeDir())
	if err != nil {
		return nil, sdkAvailable(dir)
	}
	return &dllAPI{path: p, sc: &scratch{}}, nil
}

// runningProcessNames lists executable names of running processes.
func runningProcessNames() []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []string
	if windows.Process32First(snap, &pe) != nil {
		return nil
	}
	for {
		out = append(out, windows.UTF16ToString(pe.ExeFile[:]))
		if windows.Process32Next(snap, &pe) != nil {
			break
		}
	}
	return out
}

// scratch holds out-parameters on the heap: their addresses are handed to the
// DLL as uintptr, so they must never live on a movable goroutine stack.
type scratch struct {
	u32   uint32
	u64   uint64
	h     uintptr
	count uint32
	ptr   unsafe.Pointer
	dev   DeviceInfoRaw
	item  DirItemInfo
	cap   Capacity
}

type dllAPI struct {
	path   string
	lib    windows.Handle
	procs  map[string]uintptr
	sc     *scratch
	evfStr uintptr // memory stream reused for live view
	evfImg uintptr
}

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procPeekMessage = user32.NewProc("PeekMessageW")
	procTranslate   = user32.NewProc("TranslateMessage")
	procDispatch    = user32.NewProc("DispatchMessageW")
)

// Callbacks are created once per process (syscall.NewCallback slots are
// limited). The camera reference travels through the SDK as the callback
// "context" so every event is routed to the camera it belongs to; events for a
// camera that has been closed are dropped and their object references released.
var (
	handlerMu sync.Mutex
	handlerBy = map[uintptr]handlers{}
	apiForCB  *dllAPI // the one live SDK instance (touched on the SDK thread only after Init)

	objectCB = syscall.NewCallback(func(event, object, context uintptr) uintptr {
		handlerMu.Lock()
		h, ok := handlerBy[context]
		handlerMu.Unlock()
		switch {
		case ok && h.onObject != nil:
			h.onObject(uint32(event), ref(object))
		case object != 0 && apiForCB != nil:
			// nobody owns this camera any more: release what the SDK handed us
			if uint32(event) == evObjectDirItemRequestTransfer {
				apiForCB.SkipItem(ref(object))
			} else {
				apiForCB.Release(ref(object))
			}
		}
		return 0
	})
	stateCB = syscall.NewCallback(func(event, data, context uintptr) uintptr {
		handlerMu.Lock()
		h, ok := handlerBy[context]
		handlerMu.Unlock()
		if ok && h.onState != nil {
			h.onState(uint32(event), uint32(data))
		}
		return 0
	})
)

func (d *dllAPI) Init() (err error) {
	// EDSDK needs a COM apartment on the thread that initialises it.
	// x/sys reports S_FALSE (already initialised on this thread) as an error,
	// although COM considers it a success and requires CoUninitialize.
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return fmt.Errorf("initialise Canon SDK COM apartment: %w", err)
	}
	// Any failure below undoes what was done so far: the worker only calls
	// Terminate after a successful Init, so a half-initialised SDK must not
	// leave the DLL loaded or the COM apartment open.
	defer func() {
		if err != nil {
			if d.lib != 0 {
				_ = windows.FreeLibrary(d.lib)
				d.lib = 0
			}
			d.procs = nil
			windows.CoUninitialize()
		}
	}()
	h, lerr := windows.LoadLibraryEx(d.path, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	if lerr != nil {
		return fmt.Errorf("load %s: %w", d.path, lerr)
	}
	d.lib = h
	d.procs = map[string]uintptr{}
	for _, n := range requiredExports {
		a, perr := windows.GetProcAddress(h, n)
		if perr != nil {
			return fmt.Errorf("EDSDK.dll does not export %s (wrong SDK version?): %w", n, perr)
		}
		d.procs[n] = a
	}
	if c := d.call("EdsInitializeSDK"); c != errOK {
		return &sdkError{Op: "EdsInitializeSDK", Code: c}
	}
	return nil
}

func (d *dllAPI) Terminate() {
	d.resetEvf()
	d.call("EdsTerminateSDK")
	if d.lib != 0 {
		_ = windows.FreeLibrary(d.lib)
	}
	windows.CoUninitialize()
}

func (d *dllAPI) call(name string, args ...uintptr) uint32 {
	r, _, _ := syscall.SyscallN(d.procs[name], args...)
	return uint32(r)
}

func (d *dllAPI) chk(op string, c uint32) error {
	if c != errOK {
		return &sdkError{Op: op, Code: c}
	}
	return nil
}

func (d *dllAPI) CameraList() ([]rawCamera, error) {
	s := d.sc
	if err := d.chk("EdsGetCameraList", d.call("EdsGetCameraList", uintptr(unsafe.Pointer(&s.h)))); err != nil {
		return nil, err
	}
	list := s.h
	defer d.call("EdsRelease", list)
	if err := d.chk("EdsGetChildCount", d.call("EdsGetChildCount", list, uintptr(unsafe.Pointer(&s.count)))); err != nil {
		return nil, err
	}
	n := int(s.count)
	var out []rawCamera
	for i := 0; i < n; i++ {
		if err := d.chk("EdsGetChildAtIndex", d.call("EdsGetChildAtIndex", list, uintptr(i), uintptr(unsafe.Pointer(&s.h)))); err != nil {
			return out, err
		}
		cam := s.h
		s.dev = DeviceInfoRaw{}
		if err := d.chk("EdsGetDeviceInfo", d.call("EdsGetDeviceInfo", cam, uintptr(unsafe.Pointer(&s.dev)))); err != nil {
			d.call("EdsRelease", cam)
			continue
		}
		out = append(out, rawCamera{Ref: ref(cam), Name: cstr(s.dev.DeviceDescription[:]), Port: cstr(s.dev.PortName[:])})
	}
	return out, nil
}

func (d *dllAPI) OpenSession(c ref) error {
	return d.chk("EdsOpenSession", d.call("EdsOpenSession", uintptr(c)))
}
func (d *dllAPI) CloseSession(c ref) error {
	d.resetEvf()
	return d.chk("EdsCloseSession", d.call("EdsCloseSession", uintptr(c)))
}
func (d *dllAPI) Release(r ref) {
	if r != 0 {
		d.call("EdsRelease", uintptr(r))
	}
}

func (d *dllAPI) SendCommand(c ref, cmd, param uint32) error {
	return d.chk("EdsSendCommand", d.call("EdsSendCommand", uintptr(c), uintptr(cmd), uintptr(param)))
}

func (d *dllAPI) SetU32(r ref, prop, v uint32) error {
	d.sc.u32 = v
	return d.chk("EdsSetPropertyData", d.call("EdsSetPropertyData", uintptr(r), uintptr(prop), 0, 4, uintptr(unsafe.Pointer(&d.sc.u32))))
}

func (d *dllAPI) GetU32(r ref, prop uint32) (uint32, error) {
	d.sc.u32 = 0
	err := d.chk("EdsGetPropertyData", d.call("EdsGetPropertyData", uintptr(r), uintptr(prop), 0, 4, uintptr(unsafe.Pointer(&d.sc.u32))))
	return d.sc.u32, err
}

func (d *dllAPI) AnnounceHostSpace(c ref) error {
	d.sc.cap = Capacity{NumberOfFreeClusters: 0x7FFFFFFF, BytesPerSector: 0x1000, Reset: 1}
	// EdsCapacity is passed by value; a 12-byte struct is passed by reference on x64 Windows.
	return d.chk("EdsSetCapacity", d.call("EdsSetCapacity", uintptr(c), uintptr(unsafe.Pointer(&d.sc.cap))))
}

func (d *dllAPI) resetEvf() {
	if d.evfImg != 0 {
		d.call("EdsRelease", d.evfImg)
		d.evfImg = 0
	}
	if d.evfStr != 0 {
		d.call("EdsRelease", d.evfStr)
		d.evfStr = 0
	}
}

// streamBytes copies the content of a memory stream.
func (d *dllAPI) streamBytes(stream uintptr) ([]byte, error) {
	s := d.sc
	s.ptr, s.u64 = nil, 0
	if err := d.chk("EdsGetPointer", d.call("EdsGetPointer", stream, uintptr(unsafe.Pointer(&s.ptr)))); err != nil {
		return nil, err
	}
	if err := d.chk("EdsGetLength", d.call("EdsGetLength", stream, uintptr(unsafe.Pointer(&s.u64)))); err != nil {
		return nil, err
	}
	if s.ptr == nil || s.u64 == 0 {
		return nil, nil
	}
	out := make([]byte, s.u64)
	copy(out, unsafe.Slice((*byte)(s.ptr), s.u64))
	return out, nil
}

func (d *dllAPI) DownloadEvf(c ref) ([]byte, error) {
	s := d.sc
	// Match Canon's sample: each frame gets a fresh stream and image reference.
	// Reusing a stream without resetting its position/length risks returning
	// stale bytes from the preceding frame.
	defer d.resetEvf()
	if d.evfStr == 0 {
		if err := d.chk("EdsCreateMemoryStream", d.call("EdsCreateMemoryStream", 0, uintptr(unsafe.Pointer(&s.h)))); err != nil {
			return nil, err
		}
		d.evfStr = s.h
		if err := d.chk("EdsCreateEvfImageRef", d.call("EdsCreateEvfImageRef", d.evfStr, uintptr(unsafe.Pointer(&s.h)))); err != nil {
			d.resetEvf()
			return nil, err
		}
		d.evfImg = s.h
	}
	if err := d.chk("EdsDownloadEvfImage", d.call("EdsDownloadEvfImage", uintptr(c), d.evfImg)); err != nil {
		return nil, err
	}
	return d.streamBytes(d.evfStr)
}

func (d *dllAPI) DirItem(item ref) (string, uint64, error) {
	d.sc.item = DirItemInfo{}
	if err := d.chk("EdsGetDirectoryItemInfo", d.call("EdsGetDirectoryItemInfo", uintptr(item), uintptr(unsafe.Pointer(&d.sc.item)))); err != nil {
		return "", 0, err
	}
	return cstr(d.sc.item.FileName[:]), d.sc.item.Size, nil
}

func (d *dllAPI) Download(item ref, size uint64) ([]byte, error) {
	s := d.sc
	defer d.call("EdsRelease", uintptr(item))
	if err := d.chk("EdsCreateMemoryStream", d.call("EdsCreateMemoryStream", uintptr(size), uintptr(unsafe.Pointer(&s.h)))); err != nil {
		d.call("EdsDownloadCancel", uintptr(item))
		return nil, err
	}
	stream := s.h
	defer d.call("EdsRelease", stream)
	if err := d.chk("EdsDownload", d.call("EdsDownload", uintptr(item), uintptr(size), stream)); err != nil {
		d.call("EdsDownloadCancel", uintptr(item))
		return nil, err
	}
	if err := d.chk("EdsDownloadComplete", d.call("EdsDownloadComplete", uintptr(item))); err != nil {
		return nil, err
	}
	return d.streamBytes(stream)
}

func (d *dllAPI) SkipItem(item ref) {
	d.call("EdsDownloadCancel", uintptr(item))
	d.call("EdsRelease", uintptr(item))
}

func (d *dllAPI) SetHandlers(c ref, onObject func(uint32, ref), onState func(uint32, uint32)) error {
	apiForCB = d
	handlerMu.Lock()
	handlerBy[uintptr(c)] = handlers{onObject: onObject, onState: onState}
	handlerMu.Unlock()
	if err := d.chk("EdsSetObjectEventHandler", d.call("EdsSetObjectEventHandler", uintptr(c), evAllObject, objectCB, uintptr(c))); err != nil {
		d.ClearHandlers(c)
		return err
	}
	if err := d.chk("EdsSetCameraStateEventHandler", d.call("EdsSetCameraStateEventHandler", uintptr(c), evAllState, stateCB, uintptr(c))); err != nil {
		d.ClearHandlers(c)
		return err
	}
	return nil
}

// ClearHandlers stops routing events to the camera. The SDK keeps its own
// registration until the camera reference is released, so late events still
// reach the callbacks above and are released there.
func (d *dllAPI) ClearHandlers(c ref) {
	handlerMu.Lock()
	delete(handlerBy, uintptr(c))
	handlerMu.Unlock()
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
}

func (d *dllAPI) PumpEvents() {
	d.call("EdsGetEvent")
	var m winMsg
	for i := 0; i < 64; i++ {
		r, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1 /* PM_REMOVE */)
		if r == 0 {
			return
		}
		procTranslate.Call(uintptr(unsafe.Pointer(&m)))
		procDispatch.Call(uintptr(unsafe.Pointer(&m)))
	}
}
