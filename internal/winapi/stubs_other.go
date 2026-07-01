//go:build !windows

package winapi

import "errors"

// HKL placeholder for non-Windows platforms.
type HKL uintptr

var errUnsupported = errors.New("winapi: unsupported on this platform")

func GetCursorPos() (int, int, error)         { return 0, 0, errUnsupported }
func GetCurrentKeyboardLayout() HKL           { return 0 }
func ForceENUSLayout() (HKL, error)           { return 0, errUnsupported }
func RestoreLayout(_ HKL)                     {}

// InstallMouseHook is a no-op on non-Windows platforms — there is no WH_MOUSE_LL
// equivalent, so the takeover watchdog simply never observes any real move. The
// returned uninstall func is safe to call.
func InstallMouseHook(_ func()) (func(), error) { return func() {}, nil }
