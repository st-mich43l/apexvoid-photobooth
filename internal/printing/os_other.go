//go:build !windows

package printing

// NewOSDriver returns the CUPS driver on Linux/macOS.
func NewOSDriver() Driver { return cupsDriver{} }
