package edsdk

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildDLL writes a minimal but structurally valid PE32+ DLL with the given
// machine type, import table and export table: enough for the static SDK check.
func buildDLL(machine uint16, imports, exports []string) []byte {
	const secRVA, secRaw = 0x1000, 0x400
	var sec bytes.Buffer
	pad := func(n int) int { off := sec.Len(); sec.Write(make([]byte, n)); return off }
	expOff := pad(40)
	namesOff := pad(len(exports) * 4)
	impOff := pad((len(imports) + 1) * 20)
	str := func(s string) uint32 {
		off := sec.Len()
		sec.WriteString(s)
		sec.WriteByte(0)
		return uint32(secRVA + off)
	}
	// str appends to the buffer, so the slice is re-read after every call
	for i, n := range exports {
		rva := str(n)
		binary.LittleEndian.PutUint32(sec.Bytes()[namesOff+i*4:], rva)
	}
	for i, n := range imports {
		rva := str(n)
		binary.LittleEndian.PutUint32(sec.Bytes()[impOff+i*20+12:], rva)
	}
	binary.LittleEndian.PutUint32(sec.Bytes()[expOff+24:], uint32(len(exports)))
	binary.LittleEndian.PutUint32(sec.Bytes()[expOff+32:], uint32(secRVA+namesOff))
	data := sec.Bytes()

	var f bytes.Buffer
	w := func(v any) { _ = binary.Write(&f, binary.LittleEndian, v) }
	dos := make([]byte, 0x40)
	dos[0], dos[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(dos[0x3C:], 0x40)
	f.Write(dos)
	f.WriteString("PE\x00\x00")
	w(machine)
	w(uint16(1)) // sections
	w([3]uint32{})
	w(uint16(240))
	w(uint16(0x2022))
	type dirEntry struct{ RVA, Size uint32 }
	var opt struct {
		Magic                                              uint16
		MajorLinker, MinorLinker                           uint8
		SizeOfCode, SizeOfInitData, SizeOfUninitData       uint32
		EntryPoint, BaseOfCode                             uint32
		ImageBase                                          uint64
		SectionAlignment, FileAlignment                    uint32
		MajorOS, MinorOS, MajorImage, MinorImage           uint16
		MajorSubsystem, MinorSubsystem                     uint16
		Win32Version, SizeOfImage, SizeOfHeaders, CheckSum uint32
		Subsystem, DllCharacteristics                      uint16
		StackReserve, StackCommit, HeapReserve, HeapCommit uint64
		LoaderFlags, NumberOfRvaAndSizes                   uint32
		Dirs                                               [16]dirEntry
	}
	opt.Magic, opt.ImageBase = 0x20b, 0x180000000
	opt.SectionAlignment, opt.FileAlignment = 0x1000, 0x200
	opt.MajorOS, opt.MajorSubsystem = 6, 6
	opt.SizeOfImage, opt.SizeOfHeaders, opt.Subsystem = secRVA+0x1000, secRaw, 2
	opt.NumberOfRvaAndSizes = 16
	if len(exports) > 0 {
		opt.Dirs[0] = dirEntry{uint32(secRVA + expOff), 40}
	}
	if len(imports) > 0 {
		opt.Dirs[1] = dirEntry{uint32(secRVA + impOff), uint32((len(imports) + 1) * 20)}
	}
	w(opt)
	name := [8]byte{'.', 'r', 'd', 'a', 't', 'a'}
	f.Write(name[:])
	w(uint32(len(data)))
	w(uint32(secRVA))
	raw := (len(data) + 0x1ff) &^ 0x1ff
	w(uint32(raw))
	w(uint32(secRaw))
	w([3]uint32{})
	w(uint32(0x40000040))
	for f.Len() < secRaw {
		f.WriteByte(0)
	}
	f.Write(data)
	f.Write(make([]byte, raw-len(data)))
	return f.Bytes()
}

func writeDLL(t *testing.T, path string, machine uint16, imports, exports []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buildDLL(machine, imports, exports), 0o644); err != nil {
		t.Fatal(err)
	}
}

// besideOnly resolves a dependency only if the file sits next to the SDK DLL
// (system DLLs are assumed present), like the Windows search for private DLLs.
func besideOnly(name, dir string) bool {
	if strings.EqualFold(name, "KERNEL32.dll") {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func withResolver(t *testing.T) {
	t.Helper()
	old := dllResolver
	dllResolver = besideOnly
	t.Cleanup(func() { dllResolver = old })
}

func TestInspectDistinguishesEveryProblem(t *testing.T) {
	withResolver(t)
	imps := []string{"KERNEL32.dll", "EdsImage.dll"}
	kind := func(dir string) SDKReport { return Inspect("", dir) }

	// nothing installed
	empty := t.TempDir()
	if r := kind(empty); r.Kind != SDKNotFound || r.Action == "" || len(r.Searched) == 0 {
		t.Fatalf("missing SDK: %+v", r)
	}

	// not a DLL at all
	garbage := t.TempDir()
	_ = os.WriteFile(filepath.Join(garbage, "EDSDK.dll"), []byte("this is not a dll"), 0o644)
	if r := kind(garbage); r.Kind != SDKInvalid {
		t.Fatalf("garbage: %+v", r)
	}

	// 32-bit build
	x86 := t.TempDir()
	writeDLL(t, filepath.Join(x86, "EDSDK.dll"), 0x014c, imps, requiredExports)
	if r := kind(x86); r.Kind != SDKWrongArch || !strings.Contains(r.Problem, "64-bit") || !strings.Contains(r.Action, "EDSDK_64") {
		t.Fatalf("32-bit: %+v", r)
	}

	// dependency EdsImage.dll missing
	nodep := t.TempDir()
	writeDLL(t, filepath.Join(nodep, "EDSDK.dll"), 0x8664, imps, requiredExports)
	if r := kind(nodep); r.Kind != SDKMissingDependency || len(r.MissingImports) != 1 || r.MissingImports[0] != "EdsImage.dll" {
		t.Fatalf("missing dependency: %+v", r)
	}

	// an EDSDK that lacks an entry point we call
	old := t.TempDir()
	writeDLL(t, filepath.Join(old, "EDSDK.dll"), 0x8664, imps, requiredExports[1:])
	_ = os.WriteFile(filepath.Join(old, "EdsImage.dll"), []byte("x"), 0o644)
	if r := kind(old); r.Kind != SDKMissingExport || len(r.MissingExports) != 1 || r.MissingExports[0] != requiredExports[0] {
		t.Fatalf("missing export: %+v", r)
	}

	// complete, x64
	good := t.TempDir()
	writeDLL(t, filepath.Join(good, "EDSDK.dll"), 0x8664, imps, append([]string{"EdsSomethingExtra"}, requiredExports...))
	_ = os.WriteFile(filepath.Join(good, "EdsImage.dll"), []byte("x"), 0o644)
	r := kind(good)
	if !r.OK() || r.Machine != "x64" || r.SizeBytes == 0 || filepath.Base(r.Path) != "EDSDK.dll" {
		t.Fatalf("good SDK: %+v", r)
	}
}

func TestLocateSDKUsesTheInspection(t *testing.T) {
	withResolver(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "sdk", "canon")
	writeDLL(t, filepath.Join(sub, "EDSDK.dll"), 0x014c, nil, requiredExports)
	if _, err := locateSDK("", dir); err == nil || !strings.Contains(err.Error(), "64-bit") {
		t.Fatalf("32-bit DLL must be rejected with a clear reason: %v", err)
	}
	writeDLL(t, filepath.Join(sub, "EDSDK.dll"), 0x8664, []string{"EdsImage.dll"}, requiredExports)
	if _, err := locateSDK("", dir); err == nil || !strings.Contains(err.Error(), "EdsImage.dll") {
		t.Fatalf("missing EdsImage.dll: %v", err)
	}
	_ = os.WriteFile(filepath.Join(sub, "EdsImage.dll"), []byte("x"), 0o644)
	p, err := locateSDK("", dir)
	if err != nil || filepath.Base(p) != "EDSDK.dll" {
		t.Fatalf("%v %v", p, err)
	}
	// the configured folder wins over the application folder, including Canon's own layout
	cfg := t.TempDir()
	writeDLL(t, filepath.Join(cfg, "EDSDK_64", "Dll", "EDSDK.dll"), 0x8664, nil, requiredExports)
	if p, err := locateSDK(cfg, dir); err != nil || !strings.Contains(p, "EDSDK_64") {
		t.Fatalf("configured folder not preferred: %v %v", p, err)
	}
}

func TestSDKPathPrecedenceAndPackageRoot(t *testing.T) {
	withResolver(t)
	env := t.TempDir()
	configured := t.TempDir()
	exe := t.TempDir()
	writeDLL(t, filepath.Join(env, "Windows", "EDSDK_64", "Dll", "EDSDK.dll"), 0x8664, nil, requiredExports)
	writeDLL(t, filepath.Join(configured, "EDSDK_64", "Dll", "EDSDK.dll"), 0x8664, nil, requiredExports)
	writeDLL(t, filepath.Join(exe, "EDSDK.dll"), 0x8664, nil, requiredExports)
	t.Setenv("CANON_EDSDK_PATH", env)
	if p, err := locateSDK(configured, exe); err != nil || p != filepath.Join(configured, "EDSDK_64", "Dll", "EDSDK.dll") {
		t.Fatalf("configured path should win: %s %v", p, err)
	}
	if p, err := locateSDK("", exe); err != nil || p != filepath.Join(env, "Windows", "EDSDK_64", "Dll", "EDSDK.dll") {
		t.Fatalf("environment package root should precede executable: %s %v", p, err)
	}
}
