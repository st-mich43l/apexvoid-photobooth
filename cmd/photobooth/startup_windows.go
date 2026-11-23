//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func startupError(message string) {
	title, _ := windows.UTF16PtrFromString("ApexVoid Photobooth")
	body, _ := windows.UTF16PtrFromString(message)
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")
	_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), 0x10)
}
