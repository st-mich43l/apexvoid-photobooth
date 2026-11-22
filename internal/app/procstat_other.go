//go:build !windows

package app

// processHandles is the number of OS handles held by this process (Windows only).
func processHandles() int { return 0 }
