package chromefleet

import (
	"errors"

	"github.com/tuwibu/chromekit"
)

// BrowserHandle binds a chromekit.Browser to a fleet-stable ID plus the screen
// coordinates the orchestrator uses for native cursor math. Each handle's
// Browser MUST have been launched with chromekit.WithInputBackend(BackendNative)
// AND chromekit.WithNativeWindow(X, Y, Scale) matching this struct.
type BrowserHandle struct {
	ID      string
	Browser *chromekit.Browser
	X, Y    int
	Scale   float64
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
	return nil
}
