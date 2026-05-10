//go:build windows

package winapi

import (
	"syscall"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procGetCursorPos = user32.NewProc("GetCursorPos")
)

type point struct {
	X, Y int32
}

// GetCursorPos returns the current OS cursor screen coordinates.
// Used by the dispatcher to detect human interference mid-job.
func GetCursorPos() (int, int, error) {
	var p point
	ok, _, errno := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if ok == 0 {
		return 0, 0, errno
	}
	return int(p.X), int(p.Y), nil
}
