package chromefleet

import (
	"errors"

	"github.com/tuwibu/chromekit"
)

// BrowserHandle binds a chromekit.Browser to a fleet-stable ID. When Native
// is true the Browser MUST have been launched with
// chromekit.WithInputBackend(BackendNative) AND
// chromekit.WithNativeWindow(X, Y, Scale) matching this struct — fleet routes
// Click/Type/Navigate through the single native worker with cursor drift
// checks. When Native is false (default) those actions run via CDP on the
// parallel cdp pool — no drift check, X/Y/Scale ignored.
type BrowserHandle struct {
	ID      string
	Browser *chromekit.Browser
	X, Y    int
	Scale   float64
	Native  bool
}

func (h *BrowserHandle) validate() error {
	if h == nil {
		return errors.New("BrowserHandle: nil")
	}
	if h.ID == "" {
		return errors.New("BrowserHandle: ID required")
	}
	if h.Browser == nil {
		return errors.New("BrowserHandle: Browser required")
	}
	if h.Scale <= 0 {
		h.Scale = 1.0
	}
	// Defensive: handle.Native must match Browser's actual launch backend.
	// Mismatch would cause silent drift-check failures (native handle on a
	// CDP browser) or lost anti-bot guarantees (cdp handle on native browser).
	if h.Native && h.Browser.InputBackend() != chromekit.BackendNative {
		return errors.New("BrowserHandle: Native=true but Browser launched with BackendCDP")
	}
	if !h.Native && h.Browser.InputBackend() == chromekit.BackendNative {
		return errors.New("BrowserHandle: Native=false but Browser launched with BackendNative")
	}
	return nil
}
