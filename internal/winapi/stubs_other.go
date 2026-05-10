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
