package edsdk

import "sync/atomic"

// handlers are the process-wide EDSDK callbacks (the SDK delivers them to
// plain function pointers, so they are routed through one atomic slot; only
// one camera is active at a time).
type handlers struct {
	onObject func(event uint32, item ref)
	onState  func(event, data uint32)
}

var curHandlers atomic.Pointer[handlers]

func storeHandlers(h handlers) { curHandlers.Store(&h) }
func loadHandlers() handlers {
	if p := curHandlers.Load(); p != nil {
		return *p
	}
	return handlers{}
}
