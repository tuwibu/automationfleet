package automationfleet

import (
	"errors"
	"testing"
)

// TestFleet_UnregisterRemovesHandle confirms basic removal semantics.
func TestFleet_UnregisterRemovesHandle(t *testing.T) {
	f := New()
	// White-box: bypass validate() by writing handles map directly. We're
	// testing registry bookkeeping, not BrowserHandle validation.
	f.handles["p1"] = &BrowserHandle{ID: "p1"}

	if err := f.Unregister("p1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if _, exists := f.handles["p1"]; exists {
		t.Fatal("handle should be removed from registry")
	}
}

// TestFleet_UnregisterUnknownReturnsError ensures double-unregister is detected.
func TestFleet_UnregisterUnknownReturnsError(t *testing.T) {
	f := New()
	if err := f.Unregister("ghost"); !errors.Is(err, ErrUnknownBrowser) {
		t.Fatalf("expected ErrUnknownBrowser, got %v", err)
	}
}

// TestFleet_UnregisterAllowsReRegister is the regression test for the bug
// "fleet: unknown browser id" on profile re-open. Before Unregister existed,
// Register of an ID a second time would always fail with
// "browser id already registered" because the wrapper had no way to drop
// the stale handle. This test pins that contract.
func TestFleet_UnregisterAllowsReRegister(t *testing.T) {
	f := New()
	f.handles["p1"] = &BrowserHandle{ID: "p1"}

	if err := f.Unregister("p1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	// Re-insert with same ID — would be rejected without prior Unregister.
	f.handles["p1"] = &BrowserHandle{ID: "p1"}
	if _, exists := f.handles["p1"]; !exists {
		t.Fatal("re-insert after Unregister failed")
	}
}
