package automationfleet

import "errors"

// BrowserHandle binds a Driver (chromekit or firefoxkit, via WrapChrome /
// WrapFirefox) to a fleet-stable ID. When Native is true the underlying browser
// MUST have been launched with the native input backend AND with a native
// window (X, Y, Scale) matching this struct — fleet routes Click/Type/Navigate
// through the single native worker with cursor drift checks. When Native is
// false (default) those actions run via CDP/BiDi on the parallel pool — no
// drift check, X/Y/Scale ignored.
type BrowserHandle struct {
	ID     string
	Driver Driver
	X, Y   int
	Scale  float64
	Native bool
}

func (h *BrowserHandle) validate() error {
	if h == nil {
		return errors.New("BrowserHandle: nil")
	}
	if h.ID == "" {
		return errors.New("BrowserHandle: ID required")
	}
	if h.Driver == nil {
		return errors.New("BrowserHandle: Driver required")
	}
	if h.Scale <= 0 {
		h.Scale = 1.0
	}
	// Defensive: handle.Native must match the Driver's actual launch backend.
	// Mismatch would cause silent drift-check failures (native handle on a
	// remote browser) or lost anti-bot guarantees (remote handle on native).
	if h.Native && h.Driver.InputBackend() != BackendNative {
		return errors.New("BrowserHandle: Native=true but Driver uses remote backend")
	}
	if !h.Native && h.Driver.InputBackend() == BackendNative {
		return errors.New("BrowserHandle: Native=false but Driver uses native backend")
	}
	return nil
}
