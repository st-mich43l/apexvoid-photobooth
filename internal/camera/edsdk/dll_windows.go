//go:build windows

package edsdk

import (
	"fmt"
	"os"
	"path/filepath"
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

func sdkAvailable(dir string) error {
	if _, err := locateSDK(dir, exeDir()); err != nil {
		return camera.Errf(camera.CodeSDKMissing,
			"Canon's EDSDK is proprietary and not bundled. Download it from Canon (developer registration), then copy the x64 EDSDK.dll and EdsImage.dll (and the Dll folder contents) next to the application or into ./sdk/canon, or set the folder in Settings.",
			err, "Canon EDSDK not found or not usable")
	}
	return nil
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

// callbacks are created once per process (syscall.NewCallback slots are limited).
var (
	objectCB = syscall.NewCallback(func(event, object, context uintptr) uintptr {
		if h := loadHandlers(); h.onObject != nil {
			h.onObject(uint32(event), ref(object))
		}
		return 0
	})
	stateCB = syscall.NewCallback(func(event, data, context uintptr) uintptr {
		if h := loadHandlers(); h.onState != nil {
			h.onState(uint32(event), uint32(data))
		}
		return 0
	})
)

func (d *dllAPI) Init() error {
	// EDSDK needs a COM apartment on the thread that initialises it.
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	h, err := windows.LoadLibraryEx(d.path, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	if err != nil {
		return fmt.Errorf("load %s: %w", d.path, err)
	}
	d.lib = h
	d.procs = map[string]uintptr{}
	for _, n := range []string{
		"EdsInitializeSDK", "EdsTerminateSDK", "EdsGetCameraList", "EdsGetChildCount", "EdsGetChildAtIndex", "EdsGetDeviceInfo",
		"EdsOpenSession", "EdsCloseSession", "EdsSendCommand", "EdsSetCapacity", "EdsSetPropertyData", "EdsGetPropertyData",
		"EdsCreateMemoryStream", "EdsGetPointer", "EdsGetLength", "EdsCreateEvfImageRef", "EdsDownloadEvfImage",
		"EdsGetDirectoryItemInfo", "EdsDownload", "EdsDownloadComplete", "EdsDownloadCancel", "EdsRelease", "EdsGetEvent",
		"EdsSetObjectEventHandler", "EdsSetCameraStateEventHandler",
	} {
		a, err := windows.GetProcAddress(h, n)
		if err != nil {
			return fmt.Errorf("EDSDK.dll does not export %s (wrong SDK version?): %w", n, err)
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
	storeHandlers(handlers{onObject: onObject, onState: onState})
	if err := d.chk("EdsSetObjectEventHandler", d.call("EdsSetObjectEventHandler", uintptr(c), evAllObject, objectCB, 0)); err != nil {
		return err
	}
	return d.chk("EdsSetCameraStateEventHandler", d.call("EdsSetCameraStateEventHandler", uintptr(c), evAllState, stateCB, 0))
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
