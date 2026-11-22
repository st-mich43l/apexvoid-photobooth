package edsdk

import (
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// requiredExports are the EDSDK entry points the adapter calls. They are
// checked statically (without loading the DLL) and again by GetProcAddress at
// load time.
var requiredExports = []string{
	"EdsInitializeSDK", "EdsTerminateSDK", "EdsGetCameraList", "EdsGetChildCount", "EdsGetChildAtIndex", "EdsGetDeviceInfo",
	"EdsOpenSession", "EdsCloseSession", "EdsSendCommand", "EdsSetCapacity", "EdsSetPropertyData", "EdsGetPropertyData",
	"EdsCreateMemoryStream", "EdsGetPointer", "EdsGetLength", "EdsCreateEvfImageRef", "EdsDownloadEvfImage",
	"EdsGetDirectoryItemInfo", "EdsDownload", "EdsDownloadComplete", "EdsDownloadCancel", "EdsRelease", "EdsGetEvent",
	"EdsSetObjectEventHandler", "EdsSetCameraStateEventHandler",
}

// Problem kinds reported by Inspect. They let the UI and diagnostics tell a
// missing SDK from an incompatible or incomplete one.
const (
	SDKOK                = ""
	SDKNotFound          = "not_found"
	SDKWrongArch         = "wrong_arch"
	SDKInvalid           = "invalid_dll"
	SDKMissingDependency = "missing_dependency"
	SDKMissingExport     = "missing_export"
	SDKPlatform          = "platform_unsupported"
)

// SDKReport is the result of a static, side-effect free look at the Canon SDK
// folder. Nothing is loaded or initialised.
type SDKReport struct {
	Kind           string   `json:"kind"`
	Problem        string   `json:"problem,omitempty"`
	Action         string   `json:"action,omitempty"`
	Path           string   `json:"path,omitempty"`
	Machine        string   `json:"machine,omitempty"`
	SizeBytes      int64    `json:"sizeBytes,omitempty"`
	Searched       []string `json:"searched,omitempty"`
	MissingImports []string `json:"missingImports,omitempty"`
	MissingExports []string `json:"missingExports,omitempty"`
}

// OK reports whether the SDK looks loadable.
func (r SDKReport) OK() bool { return r.Kind == SDKOK }

// dllResolver reports whether a dependent DLL can be found (beside the SDK DLL
// or in the Windows search path). Replaced in tests; the Windows build checks
// the real search path, other platforms cannot and assume it is present.
var dllResolver = resolveDependency

// Inspect locates EDSDK.dll and checks architecture, dependencies and exports.
func Inspect(configured, exeDir string) SDKReport {
	cands := sdkCandidates(configured, exeDir)
	rep := SDKReport{Searched: cands}
	var firstProblem *SDKReport
	for _, d := range cands {
		p := filepath.Join(d, "EDSDK.dll")
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		r := inspectOne(p, d)
		r.SizeBytes, r.Searched = st.Size(), cands
		if r.OK() {
			return r
		}
		if firstProblem == nil {
			firstProblem = &r
		}
	}
	if firstProblem != nil {
		return *firstProblem
	}
	rep.Kind = SDKNotFound
	rep.Problem = fmt.Sprintf("EDSDK.dll not found (searched %d locations)", len(cands))
	rep.Action = "Copy the 64-bit EDSDK.dll and EdsImage.dll (and the rest of Canon's Dll folder) next to the application, into .\\sdk\\canon, or choose the folder in Settings."
	return rep
}

func inspectOne(path, dir string) SDKReport {
	r := SDKReport{Path: path}
	f, err := pe.Open(path)
	if err != nil {
		r.Kind, r.Problem = SDKInvalid, fmt.Sprintf("%s is not a valid DLL: %v", path, err)
		r.Action = "Copy EDSDK.dll again from Canon's SDK package; the file may be damaged or not a DLL."
		return r
	}
	defer f.Close()
	r.Machine = machineName(f.Machine)
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		r.Kind, r.Problem = SDKWrongArch, fmt.Sprintf("%s is a %s DLL; this application is 64-bit", path, r.Machine)
		r.Action = "Use the files from Canon's EDSDK_64 folder (the x64 build), not the 32-bit ones."
		return r
	}
	imps, ierr := importedDLLs(f)
	if ierr != nil {
		r.Kind, r.Problem = SDKInvalid, fmt.Sprintf("%s has an unreadable import table: %v", path, ierr)
		r.Action = "Copy EDSDK.dll again from Canon's SDK package."
		return r
	}
	for _, n := range imps {
		if !dllResolver(n, dir) {
			r.MissingImports = append(r.MissingImports, n)
		}
	}
	if len(r.MissingImports) > 0 {
		sort.Strings(r.MissingImports)
		r.Kind, r.Problem = SDKMissingDependency, "EDSDK.dll needs "+strings.Join(r.MissingImports, ", ")+" which cannot be found"
		r.Action = "Copy the complete Canon Dll folder (EdsImage.dll and the other files beside EDSDK.dll) and install the Microsoft Visual C++ Redistributable (x64) if a vcruntime/msvcp file is named."
		return r
	}
	exps, eerr := exportedNames(f)
	if eerr != nil {
		r.Kind, r.Problem = SDKInvalid, fmt.Sprintf("%s has an unreadable export table: %v", path, eerr)
		r.Action = "Copy EDSDK.dll again from Canon's SDK package."
		return r
	}
	have := map[string]bool{}
	for _, n := range exps {
		have[n] = true
	}
	for _, n := range requiredExports {
		if !have[n] {
			r.MissingExports = append(r.MissingExports, n)
		}
	}
	if len(r.MissingExports) > 0 {
		r.Kind, r.Problem = SDKMissingExport, "EDSDK.dll does not export "+strings.Join(r.MissingExports, ", ")
		r.Action = "This is not a compatible EDSDK build. Use EDSDK 13.x for Windows x64 from Canon."
	}
	return r
}

func machineName(m uint16) string {
	switch m {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return "x64"
	case pe.IMAGE_FILE_MACHINE_I386:
		return "x86 (32-bit)"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return "ARM64"
	}
	return fmt.Sprintf("machine 0x%x", m)
}

// rvaReader maps relative virtual addresses to bytes of the file.
type rvaReader struct{ f *pe.File }

func (r rvaReader) bytes(rva uint32, max int) ([]byte, error) {
	for _, s := range r.f.Sections {
		if rva >= s.VirtualAddress && rva < s.VirtualAddress+max32(s.VirtualSize, s.Size) {
			d, err := s.Data()
			if err != nil {
				return nil, err
			}
			off := int(rva - s.VirtualAddress)
			if off > len(d) {
				return nil, errors.New("address beyond section data")
			}
			d = d[off:]
			if max > 0 && len(d) > max {
				d = d[:max]
			}
			return d, nil
		}
	}
	return nil, fmt.Errorf("address 0x%x is not inside any section", rva)
}

func max32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func (r rvaReader) cstring(rva uint32) (string, error) {
	b, err := r.bytes(rva, 512)
	if err != nil {
		return "", err
	}
	for i, c := range b {
		if c == 0 {
			return string(b[:i]), nil
		}
	}
	return "", errors.New("unterminated string")
}

func dataDir(f *pe.File, idx int) (rva, size uint32) {
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		if idx < len(oh.DataDirectory) {
			return oh.DataDirectory[idx].VirtualAddress, oh.DataDirectory[idx].Size
		}
	case *pe.OptionalHeader32:
		if idx < len(oh.DataDirectory) {
			return oh.DataDirectory[idx].VirtualAddress, oh.DataDirectory[idx].Size
		}
	}
	return 0, 0
}

// importedDLLs lists the DLL names in the import table (Go's debug/pe does not).
func importedDLLs(f *pe.File) ([]string, error) {
	rva, _ := dataDir(f, pe.IMAGE_DIRECTORY_ENTRY_IMPORT)
	if rva == 0 {
		return nil, nil
	}
	rd := rvaReader{f}
	var out []string
	for i := 0; i < 512; i++ {
		d, err := rd.bytes(rva+uint32(i)*20, 20)
		if err != nil {
			return out, err
		}
		if len(d) < 20 {
			return out, errors.New("truncated import descriptor")
		}
		name := binary.LittleEndian.Uint32(d[12:16])
		if name == 0 {
			break
		}
		s, err := rd.cstring(name)
		if err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}

// exportedNames lists the named exports.
func exportedNames(f *pe.File) ([]string, error) {
	rva, _ := dataDir(f, pe.IMAGE_DIRECTORY_ENTRY_EXPORT)
	if rva == 0 {
		return nil, nil
	}
	rd := rvaReader{f}
	h, err := rd.bytes(rva, 40)
	if err != nil || len(h) < 40 {
		return nil, fmt.Errorf("export directory: %v", err)
	}
	n := binary.LittleEndian.Uint32(h[24:28])
	namesRVA := binary.LittleEndian.Uint32(h[32:36])
	if n > 20000 {
		return nil, errors.New("implausible export count")
	}
	tbl, err := rd.bytes(namesRVA, int(n)*4)
	if err != nil || len(tbl) < int(n)*4 {
		return nil, fmt.Errorf("export name table: %v", err)
	}
	out := make([]string, 0, n)
	for i := 0; i < int(n); i++ {
		s, err := rd.cstring(binary.LittleEndian.Uint32(tbl[i*4:]))
		if err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}
