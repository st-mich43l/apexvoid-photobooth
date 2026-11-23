//go:build windows

package printing

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // decoders for rendered print files
	_ "image/png"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// winDriver prints through the Windows print spooler and the installed printer
// driver: the image is decoded, placed according to the printer's real page
// geometry and drawn with GDI (StretchDIBits) between StartDoc/EndDoc. Raw JPEG
// bytes are never sent to the printer. No custom driver is involved.
type winDriver struct{}

func (winDriver) Name() string { return "windows-spooler (GDI)" }

var (
	winspool               = windows.NewLazySystemDLL("winspool.drv")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procEnumPrinters       = winspool.NewProc("EnumPrintersW")
	procGetDefaultPrinter  = winspool.NewProc("GetDefaultPrinterW")
	procOpenPrinter        = winspool.NewProc("OpenPrinterW")
	procClosePrinter       = winspool.NewProc("ClosePrinter")
	procGetJob             = winspool.NewProc("GetJobW")
	procDeviceCapabilities = winspool.NewProc("DeviceCapabilitiesW")
	procDocumentProperties = winspool.NewProc("DocumentPropertiesW")
	procCreateDC           = gdi32.NewProc("CreateDCW")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procStartDoc           = gdi32.NewProc("StartDocW")
	procEndDoc             = gdi32.NewProc("EndDoc")
	procAbortDoc           = gdi32.NewProc("AbortDoc")
	procStartPage          = gdi32.NewProc("StartPage")
	procEndPage            = gdi32.NewProc("EndPage")
	procGetDeviceCaps      = gdi32.NewProc("GetDeviceCaps")
	procStretchDIBits      = gdi32.NewProc("StretchDIBits")
	procSetStretchBltMode  = gdi32.NewProc("SetStretchBltMode")
)

const (
	printerEnumLocal       = 0x2
	printerEnumConnections = 0x4

	dcPapers          = 2
	dcPaperSize       = 3
	dcPaperNames      = 16
	dcOrientation     = 17
	dcCopies          = 18
	dcEnumResolutions = 13
	dcColorDevice     = 32

	dmOutBuffer = 2
	dmInBuffer  = 8

	dmOrientation = 0x1
	dmPaperSize   = 0x2
	dmPaperLength = 0x4
	dmPaperWidth  = 0x8
	dmCopies      = 0x100
	dmUserPaper   = 256 // DMPAPER_USER: custom size from dmPaperLength/Width

	horzres         = 8
	vertres         = 10
	logpixelsx      = 88
	logpixelsy      = 90
	physicalwidth   = 110
	physicalheight  = 111
	physicaloffsetx = 112
	physicaloffsety = 113

	halftone            = 4
	srccopy             = 0x00CC0020
	dibRGBColors        = 0
	errInvalidParameter = 87
)

// printerInfo2 is PRINTER_INFO_2W.
type printerInfo2 struct {
	pServerName, pPrinterName, pShareName, pPortName, pDriverName, pComment, pLocation     *uint16
	pDevMode                                                                               uintptr
	pSepFile, pPrintProcessor, pDatatype, pParameters                                      *uint16
	pSecurityDescriptor                                                                    uintptr
	Attributes, Priority, DefaultPriority, StartTime, UntilTime, Status, cJobs, AveragePPM uint32
}

// jobInfo2 is JOB_INFO_2W.
type jobInfo2 struct {
	JobID                                                                                                               uint32
	pPrinterName, pMachineName, pUserName, pDocument, pNotifyName, pDatatype, pPrintProcessor, pParameters, pDriverName *uint16
	pDevMode                                                                                                            uintptr
	pStatus                                                                                                             *uint16
	pSecurityDescriptor                                                                                                 uintptr
	Status, Priority, Position, StartTime, UntilTime, TotalPages, Size                                                  uint32
	Submitted                                                                                                           [8]uint16
	Time, PagesPrinted                                                                                                  uint32
}

type docInfo struct {
	cbSize                                int32
	lpszDocName, lpszOutput, lpszDatatype *uint16
	fwType                                uint32
}

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes, Bits  uint16
	Compression   uint32
	SizeImage     uint32
	XPels, YPels  int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// DEVMODEW field offsets (the structure is handled as bytes because its size is
// driver specific: dmSize + dmDriverExtra).
const (
	offDmFields      = 72
	offDmOrientation = 76
	offDmPaperSize   = 78
	offDmPaperLength = 80
	offDmPaperWidth  = 82
	offDmCopies      = 86
)

func putU16(b []byte, off int, v uint16) { b[off], b[off+1] = byte(v), byte(v>>8) }
func getU16(b []byte, off int) uint16    { return uint16(b[off]) | uint16(b[off+1])<<8 }
func putU32(b []byte, off int, v uint32) {
	b[off], b[off+1], b[off+2], b[off+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}
func getU32(b []byte, off int) uint32 {
	return uint32(b[off]) | uint32(b[off+1])<<8 | uint32(b[off+2])<<16 | uint32(b[off+3])<<24
}

func u16(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func lastErr(e error, op string) error {
	var en syscall.Errno
	if errors.As(e, &en) && en != 0 {
		return fmt.Errorf("%s failed: %w", op, en)
	}
	return fmt.Errorf("%s failed", op)
}

// installed returns all printers (local + connections) with their raw info.
func installed() ([]printerInfo2, error) {
	flags := uintptr(printerEnumLocal | printerEnumConnections)
	var needed, returned uint32
	procEnumPrinters.Call(flags, 0, 2, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return nil, nil // no printers installed
	}
	buf := make([]byte, needed)
	r, _, e := procEnumPrinters.Call(flags, 0, 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r == 0 {
		return nil, lastErr(e, "EnumPrinters")
	}
	infos := unsafe.Slice((*printerInfo2)(unsafe.Pointer(&buf[0])), int(returned))
	out := make([]printerInfo2, len(infos))
	// the strings live inside buf: copy them out before buf can be collected
	for i, in := range infos {
		out[i] = in
		out[i].pPrinterName = u16(windows.UTF16PtrToString(in.pPrinterName))
		out[i].pPortName = u16(windows.UTF16PtrToString(in.pPortName))
		out[i].pDriverName = u16(windows.UTF16PtrToString(in.pDriverName))
		out[i].pDevMode = 0
	}
	runtime.KeepAlive(buf)
	return out, nil
}

func defaultPrinter() string {
	var n uint32
	procGetDefaultPrinter.Call(0, uintptr(unsafe.Pointer(&n)))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	if r, _, _ := procGetDefaultPrinter.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func (winDriver) List(ctx context.Context) ([]Printer, error) {
	infos, err := installed()
	if err != nil {
		return nil, err
	}
	def := defaultPrinter()
	out := make([]Printer, 0, len(infos))
	for _, in := range infos {
		name := windows.UTF16PtrToString(in.pPrinterName)
		st, detail := MapPrinterStatus(in.Status, in.Attributes)
		out = append(out, Printer{Name: name, Default: strings.EqualFold(name, def), Status: st, Detail: detail,
			Port: windows.UTF16PtrToString(in.pPortName), Driver: windows.UTF16PtrToString(in.pDriverName)})
	}
	return out, nil
}

func portOf(printer string) (string, error) {
	infos, err := installed()
	if err != nil {
		return "", err
	}
	for _, in := range infos {
		if windows.UTF16PtrToString(in.pPrinterName) == printer {
			return windows.UTF16PtrToString(in.pPortName), nil
		}
	}
	return "", fmt.Errorf("printer %q is not installed", printer)
}

// caps wraps DeviceCapabilitiesW; ok=false when the driver does not answer.
func caps(printer, port string, what uintptr, out unsafe.Pointer) (int, bool) {
	r, _, _ := procDeviceCapabilities.Call(uintptr(unsafe.Pointer(u16(printer))), uintptr(unsafe.Pointer(u16(port))), what, uintptr(out), 0)
	n := int(int32(r))
	return n, n >= 0
}

func (winDriver) Capabilities(ctx context.Context, printer string) (Caps, error) {
	port, err := portOf(printer)
	if err != nil {
		return Caps{}, err
	}
	var c Caps
	n, ok := caps(printer, port, dcPapers, nil)
	if ok && n > 0 {
		ids := make([]uint16, n)
		names := make([]uint16, n*64)
		sizes := make([]int32, n*2) // POINT: tenths of a millimetre
		caps(printer, port, dcPapers, unsafe.Pointer(&ids[0]))
		caps(printer, port, dcPaperNames, unsafe.Pointer(&names[0]))
		caps(printer, port, dcPaperSize, unsafe.Pointer(&sizes[0]))
		for i := 0; i < n; i++ {
			nm := windows.UTF16ToString(names[i*64 : (i+1)*64])
			w, h := float64(sizes[i*2])/10/25.4, float64(sizes[i*2+1])/10/25.4
			if nm == "" || w <= 0 || h <= 0 {
				continue
			}
			c.Papers = append(c.Papers, Paper{ID: int(ids[i]), Name: nm, WidthIn: round3(w), HeightIn: round3(h)})
		}
	}
	if len(c.Papers) == 0 {
		c.Papers, c.Note = KnownPapers, "The driver did not report its paper sizes; a generic list is shown."
	}
	c.Orientations = []Orientation{Portrait}
	if o, ok := caps(printer, port, dcOrientation, nil); ok && o != 0 {
		c.Orientations = append(c.Orientations, Landscape)
	}
	c.MaxCopies = 10
	if n, ok := caps(printer, port, dcCopies, nil); ok && n > 0 && n < 10 {
		c.MaxCopies = n
	}
	if n, ok := caps(printer, port, dcColorDevice, nil); ok {
		c.Color = n == 1
	}
	if n, ok := caps(printer, port, dcEnumResolutions, nil); ok && n > 0 {
		res := make([]int32, n*2)
		caps(printer, port, dcEnumResolutions, unsafe.Pointer(&res[0]))
		for i := 0; i < n; i++ {
			c.Resolutions = append(c.Resolutions, [2]int{int(res[i*2]), int(res[i*2+1])})
		}
	}
	return c, nil
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

func openPrinter(name string) (uintptr, error) {
	var h uintptr
	r, _, e := procOpenPrinter.Call(uintptr(unsafe.Pointer(u16(name))), uintptr(unsafe.Pointer(&h)), 0)
	if r == 0 {
		return 0, lastErr(e, "OpenPrinter")
	}
	return h, nil
}

// devmode asks the driver for its DEVMODE, applies paper/orientation/copies and
// lets the driver validate the result.
func devmode(printer string, p Paper, o Orientation, copies int) ([]byte, error) {
	h, err := openPrinter(printer)
	if err != nil {
		return nil, err
	}
	defer procClosePrinter.Call(h)
	pn := uintptr(unsafe.Pointer(u16(printer)))
	size, _, _ := procDocumentProperties.Call(0, h, pn, 0, 0, 0)
	if int32(size) <= 0 {
		return nil, fmt.Errorf("the driver for %q returned no print settings", printer)
	}
	buf := make([]byte, size)
	if r, _, _ := procDocumentProperties.Call(0, h, pn, uintptr(unsafe.Pointer(&buf[0])), 0, dmOutBuffer); int32(r) != 1 {
		return nil, fmt.Errorf("could not read the default print settings of %q", printer)
	}
	fields := getU32(buf, offDmFields)
	if o == Landscape || o == Portrait {
		v := uint16(1)
		if o == Landscape {
			v = 2
		}
		putU16(buf, offDmOrientation, v)
		fields |= dmOrientation
	}
	if p.ID > 0 {
		putU16(buf, offDmPaperSize, uint16(p.ID))
		fields |= dmPaperSize
	} else if p.WidthIn > 0 && p.HeightIn > 0 {
		putU16(buf, offDmPaperSize, dmUserPaper)
		putU16(buf, offDmPaperWidth, uint16(p.WidthIn*254+0.5)) // tenths of a millimetre
		putU16(buf, offDmPaperLength, uint16(p.HeightIn*254+0.5))
		fields |= dmPaperSize | dmPaperWidth | dmPaperLength
	}
	if copies > 0 {
		putU16(buf, offDmCopies, uint16(copies))
		fields |= dmCopies
	}
	putU32(buf, offDmFields, fields)
	if r, _, _ := procDocumentProperties.Call(0, h, pn, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&buf[0])), dmInBuffer|dmOutBuffer); int32(r) != 1 {
		return nil, fmt.Errorf("the printer driver rejected paper %q / %s", p.Name, o)
	}
	return buf, nil
}

func createDC(printer string, dm []byte) (uintptr, error) {
	var pdm uintptr
	if len(dm) > 0 {
		pdm = uintptr(unsafe.Pointer(&dm[0]))
	}
	hdc, _, e := procCreateDC.Call(uintptr(unsafe.Pointer(u16("WINSPOOL"))), uintptr(unsafe.Pointer(u16(printer))), 0, pdm)
	if hdc == 0 {
		return 0, lastErr(e, "CreateDC")
	}
	return hdc, nil
}

func devCap(hdc uintptr, idx int) int {
	r, _, _ := procGetDeviceCaps.Call(hdc, uintptr(idx))
	return int(int32(r))
}

func geometryOf(hdc uintptr) PageGeometry {
	return PageGeometry{PhysW: devCap(hdc, physicalwidth), PhysH: devCap(hdc, physicalheight), PrintW: devCap(hdc, horzres), PrintH: devCap(hdc, vertres),
		OffX: devCap(hdc, physicaloffsetx), OffY: devCap(hdc, physicaloffsety), DPIX: devCap(hdc, logpixelsx), DPIY: devCap(hdc, logpixelsy)}
}

func (winDriver) Geometry(ctx context.Context, printer string, p Paper, o Orientation) (PageGeometry, error) {
	if o == Auto {
		o = Portrait
	}
	dm, err := devmode(printer, p, o, 1)
	if err != nil {
		return PageGeometry{}, err
	}
	hdc, err := createDC(printer, dm)
	if err != nil {
		return PageGeometry{}, err
	}
	defer procDeleteDC.Call(hdc)
	g := geometryOf(hdc)
	if g.PhysW <= 0 || g.PhysH <= 0 || g.DPIX <= 0 {
		return PageGeometry{}, fmt.Errorf("the driver for %q reported an empty page", printer)
	}
	return g, nil
}

func (winDriver) Submit(ctx context.Context, r SubmitRequest) (string, error) {
	runtime.LockOSThread() // GDI printing: one OS thread from StartDoc to EndDoc
	defer runtime.UnlockOSThread()

	f, err := os.Open(r.File)
	if err != nil {
		return "", fmt.Errorf("the print file cannot be read: %w", err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return "", fmt.Errorf("the print file is not a readable image: %w", err)
	}
	b := img.Bounds()
	orient := r.Orientation
	if orient == Auto || orient == "" {
		orient = Portrait
		if b.Dx() > b.Dy() {
			orient = Landscape
		}
	}
	// one document with N pages is deterministic across drivers (dmCopies is not)
	dm, err := devmode(r.Printer, r.Paper, orient, 1)
	if err != nil {
		return "", err
	}
	hdc, err := createDC(r.Printer, dm)
	if err != nil {
		return "", err
	}
	defer procDeleteDC.Call(hdc)
	g := geometryOf(hdc)
	dpi := r.ImageDPI
	if dpi <= 0 {
		dpi = 300
	}
	fit := r.Fit
	if fit == "" {
		fit = FitPrintable
	}
	pl := PlanPage(b.Dx(), b.Dy(), dpi, g, fit)
	if pl.Rotated {
		img = Rotate90(img)
	}
	pix, w, h := ToBGRA(img)
	bmi := bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}

	title := r.Title
	if title == "" {
		title = "ApexVoid Photobooth"
	}
	di := docInfo{cbSize: int32(unsafe.Sizeof(docInfo{})), lpszDocName: u16(title)}
	jobID, _, e := procStartDoc.Call(hdc, uintptr(unsafe.Pointer(&di)))
	if int32(jobID) <= 0 {
		return "", lastErr(e, "StartDoc (the printer driver or spooler refused the job)")
	}
	nativeID := strconv.Itoa(int(int32(jobID)))
	copies := r.Copies
	if copies < 1 {
		copies = 1
	}
	for c := 0; c < copies; c++ {
		if ctx.Err() != nil {
			procAbortDoc.Call(hdc)
			return nativeID, ctx.Err()
		}
		if rr, _, e := procStartPage.Call(hdc); int32(rr) <= 0 {
			procAbortDoc.Call(hdc)
			return nativeID, lastErr(e, "StartPage")
		}
		procSetStretchBltMode.Call(hdc, halftone)
		rr, _, _ := procStretchDIBits.Call(hdc,
			uintptr(pl.Dest.Min.X), uintptr(pl.Dest.Min.Y), uintptr(pl.Dest.Dx()), uintptr(pl.Dest.Dy()),
			uintptr(pl.Src.Min.X), uintptr(pl.Src.Min.Y), uintptr(pl.Src.Dx()), uintptr(pl.Src.Dy()),
			uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&bmi)), dibRGBColors, srccopy)
		if int32(rr) <= 0 { // 0 or GDI_ERROR (-1)
			procAbortDoc.Call(hdc)
			return nativeID, errors.New("drawing the image onto the printer page failed")
		}
		if rr, _, e := procEndPage.Call(hdc); int32(rr) <= 0 {
			procAbortDoc.Call(hdc)
			return nativeID, lastErr(e, "EndPage")
		}
	}
	runtime.KeepAlive(pix)
	if rr, _, e := procEndDoc.Call(hdc); int32(rr) <= 0 {
		return nativeID, lastErr(e, "EndDoc")
	}
	return nativeID, nil
}

func (winDriver) Status(ctx context.Context, printer, nativeID string) (JobState, error) {
	id, err := strconv.Atoi(nativeID)
	if err != nil {
		return JobState{}, fmt.Errorf("bad job id %q", nativeID)
	}
	h, err := openPrinter(printer)
	if err != nil {
		return JobState{}, err
	}
	defer procClosePrinter.Call(h)
	var needed uint32
	procGetJob.Call(h, uintptr(id), 2, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if needed == 0 {
		return MapSpoolerJob(0, false), nil // not in the queue (any more)
	}
	buf := make([]byte, needed)
	r, _, e := procGetJob.Call(h, uintptr(id), 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		var en syscall.Errno
		if errors.As(e, &en) && en == errInvalidParameter {
			return MapSpoolerJob(0, false), nil
		}
		return JobState{}, lastErr(e, "GetJob")
	}
	ji := (*jobInfo2)(unsafe.Pointer(&buf[0]))
	st := MapSpoolerJob(ji.Status, true)
	st.Pages = int(ji.PagesPrinted)
	if s := windows.UTF16PtrToString(ji.pStatus); s != "" {
		st.Detail += " (" + s + ")"
	}
	runtime.KeepAlive(buf)
	return st, nil
}
