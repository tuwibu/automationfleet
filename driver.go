package automationfleet

import (
	"context"
	"time"
)

// Backend is a fleet-internal enum identifying how a Driver dispatches input.
// It does NOT map 1:1 to any kit's integer values — chromekit uses
// BackendCDP=0/BackendNative=1, firefoxkit uses BackendBiDi=0/BackendNative=1.
// Adapters MUST translate via the named constants below; never cast a kit's
// backend int directly to Backend.
type Backend int

const (
	// BackendNative drives the OS pointer/keyboard directly (winapi on Windows).
	BackendNative Backend = iota
	// BackendRemote dispatches input over the wire — CDP (chrome) or BiDi (firefox).
	BackendRemote
)

// BoundingBox is a viewport-relative box in CSS pixels, top-left origin.
type BoundingBox struct {
	X, Y, Width, Height float64
}

// Center returns the box center in CSS pixels.
func (b BoundingBox) Center() (float64, float64) {
	return b.X + b.Width/2, b.Y + b.Height/2
}

// Driver is a browser the fleet can drive, independent of the underlying kit
// (chromekit / firefoxkit). Adapters wrap a concrete kit Browser into this
// interface — see WrapChrome / WrapFirefox.
type Driver interface {
	// Current returns the active page, or nil when no page is live. Adapters
	// MUST return a nil interface (not a non-nil interface wrapping a nil
	// pointer) so callers' `page == nil` checks work.
	Current() Page
	Focus(ctx context.Context) error
	InputBackend() Backend
	ContentOffset() (int, int, error)
}

// Page is the subset of a kit Page the dispatcher drives.
type Page interface {
	Mouse() Mouse
	Keyboard() Keyboard
	Evaluate(expr string, out any) error
	Navigate(url string, timeout time.Duration) error
	// WaitForSelector blocks until the element appears (or timeout). Abstracts
	// the kits' differing QuerySelector return types (*cdp.Node vs *ElementHandle).
	WaitForSelector(selector string, timeout time.Duration) error
	// ElementCenter scrolls the element into view, re-queries its box, and
	// returns the center in CSS pixels (viewport-relative). Adapters hide the
	// per-kit scrollIntoView + bounding-box dance.
	ElementCenter(selector string) (x, y float64, err error)
}

// Mouse mirrors the kits' Mouse API surface the dispatcher uses.
type Mouse interface {
	MoveTo(x, y float64) error
	ClickAt(x, y float64) error
	Click(selector string) error
	FocusElement(selector string) error
}

// Keyboard mirrors the kits' Keyboard API surface the dispatcher uses.
type Keyboard interface {
	ClearInput(selector ...string) error
	TypeHuman(text string) error
}
