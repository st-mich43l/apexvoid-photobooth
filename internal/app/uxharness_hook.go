//go:build uxharness

package app

// DevSetCameraPresent plugs/unplugs the simulated camera. Only compiled into the
// screenshot harness (cmd/uxharness); it is a package function, not a bound method.
func DevSetCameraPresent(s *Service, present bool) { s.mockDriver().SetPresent(present) }
