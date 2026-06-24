//go:build !windows

package automationfleet

import "context"

// HotkeyBinding mirrors the Windows type so callers compile cross-platform.
type HotkeyBinding struct {
	Hotkey Hotkey
	OnFire func()
}

// runHotkeyListener is a no-op outside Windows. The hotkey path relies on
// RegisterHotKey + GetMessage which have no portable equivalent.
func runHotkeyListener(ctx context.Context, _ Hotkey, _ func()) error {
	<-ctx.Done()
	return nil
}

// runHotkeyMultiListener is a no-op outside Windows.
func runHotkeyMultiListener(ctx context.Context, _ []HotkeyBinding) error {
	<-ctx.Done()
	return nil
}
