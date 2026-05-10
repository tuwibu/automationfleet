//go:build windows

package winapi

import (
	"syscall"
	"unsafe"
)

var (
	procGetKeyboardLayout      = user32.NewProc("GetKeyboardLayout")
	procActivateKeyboardLayout = user32.NewProc("ActivateKeyboardLayout")
	procLoadKeyboardLayoutW    = user32.NewProc("LoadKeyboardLayoutW")
)

const (
	klfActivate = 0x00000001
	enUSLayout  = "00000409"
)

// HKL is an opaque keyboard layout handle.
type HKL uintptr

// GetCurrentKeyboardLayout returns the layout active on the calling thread.
func GetCurrentKeyboardLayout() HKL {
	h, _, _ := procGetKeyboardLayout.Call(0)
	return HKL(h)
}

// ForceENUSLayout switches the calling thread to en-US, returning the previous
// layout so the caller can restore it. Mitigates IME / non-Latin layouts
// scrambling typed text. Returns 0 + error on failure.
func ForceENUSLayout() (HKL, error) {
	prev := GetCurrentKeyboardLayout()
	name, err := syscall.UTF16PtrFromString(enUSLayout)
	if err != nil {
		return prev, err
	}
	hkl, _, errno := procLoadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(name)), klfActivate)
	if hkl == 0 {
		return prev, errno
	}
	procActivateKeyboardLayout.Call(hkl, klfActivate)
	return prev, nil
}

// RestoreLayout reactivates the supplied layout. No-op on zero.
func RestoreLayout(prev HKL) {
	if prev == 0 {
		return
	}
	procActivateKeyboardLayout.Call(uintptr(prev), klfActivate)
}
