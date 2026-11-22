package edsdk

// handlers are the callbacks of one camera. The Windows build routes SDK
// events to them by camera reference (see dll_windows.go).
type handlers struct {
	onObject func(event uint32, item ref)
	onState  func(event, data uint32)
}
