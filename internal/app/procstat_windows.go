//go:build windows

package app

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetProcessHandleCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

// processHandles is the number of OS handles held by this process; a count that
// keeps growing across repeated sessions points at a leaked camera/SDK handle.
func processHandles() int {
	var n uint32
	h, err := windows.GetCurrentProcess()
	if err != nil {
		return 0
	}
	if r, _, _ := procGetProcessHandleCount.Call(uintptr(h), uintptr(unsafe.Pointer(&n))); r == 0 {
		return 0
	}
	return int(n)
}
