//go:build windows

package printing

// NewOSDriver returns the native Windows spooler/GDI driver.
func NewOSDriver() Driver { return winDriver{} }
