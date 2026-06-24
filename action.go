package automationfleet

import (
	"errors"
	"time"
)

// Action describes one piece of work the dispatcher executes against a single
// browser. Implementations are simple value types — the dispatcher type-switches
// to pick the right execution path (native vs CDP-only).
type Action interface {
	kind() string
	validate() error
}

// ClickAction performs scroll-into-view → re-query bbox → move cursor → click.
// The whole sequence runs inside the native critical section.
type ClickAction struct {
	Selector string
	Button   MouseButton
}

func (ClickAction) kind() string { return "click" }
func (a ClickAction) validate() error {
	if a.Selector == "" {
		return errors.New("ClickAction: Selector required")
	}
	return nil
}

// TypeAction clicks the target first, then types text via native input.
// Selector is mandatory — typing without a focused target risks the
// keystrokes landing in another window.
//
// ClearFirst, when true, wipes any existing value via Ctrl+A then Delete
// (chromekit Keyboard().ClearInput) after the click and before typing —
// safer than relying on the input being empty.
type TypeAction struct {
	Selector   string
	Text       string
	ClearFirst bool
}

func (TypeAction) kind() string { return "type" }
func (a TypeAction) validate() error {
	if a.Selector == "" {
		return errors.New("TypeAction: Selector required (typing without target leaks to wrong window)")
	}
	if a.Text == "" {
		return errors.New("TypeAction: Text required")
	}
	return nil
}

// NavigateAction drives the browser through Chrome's omnibox (native input
// path: Ctrl+L → Ctrl+A → type URL → End → Enter). Runs in the native
// critical section because typing leaks into whatever window has focus.
// Use this for stress tests that exercise omnibox alongside click/type.
type NavigateAction struct {
	URL     string
	Timeout time.Duration // 0 → fleet default
}

func (NavigateAction) kind() string { return "navigate" }
func (a NavigateAction) validate() error {
	if a.URL == "" {
		return errors.New("NavigateAction: URL required")
	}
	return nil
}

// ScrollAction scrolls the viewport without clicking. Runs CDP-only — no
// native cursor involvement, parallel-safe with other browsers.
type ScrollAction struct {
	Selector string  // optional; if empty, scroll the page
	DeltaY   float64 // pixels; negative = up
}

func (ScrollAction) kind() string  { return "scroll" }
func (ScrollAction) validate() error { return nil }

// WaitAction waits for an element to appear (or disappear). CDP-only.
type WaitAction struct {
	Selector string
	Timeout  time.Duration
}

func (WaitAction) kind() string { return "wait" }
func (a WaitAction) validate() error {
	if a.Selector == "" {
		return errors.New("WaitAction: Selector required")
	}
	if a.Timeout <= 0 {
		return errors.New("WaitAction: Timeout > 0 required")
	}
	return nil
}

// MouseButton mirrors chromekit/input.MouseButton for fleet-level API surface.
type MouseButton int

const (
	MouseLeft MouseButton = iota
	MouseRight
	MouseMiddle
)

// needsNativeCriticalSection returns true if the action drives the OS cursor /
// keyboard and must therefore run on the single native worker.
func needsNativeCriticalSection(a Action) bool {
	switch a.(type) {
	case ClickAction, TypeAction, NavigateAction:
		return true
	default:
		return false
	}
}
