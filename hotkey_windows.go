//go:build windows

package chromefleet

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
)

const (
	wmHotkey = 0x0312
	wmQuit   = 0x0012
)

type msg struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	_       uint32
}

// HotkeyBinding pairs a global hotkey combo with the callback to fire when it
// hits. Used by runHotkeyMultiListener.
type HotkeyBinding struct {
	Hotkey Hotkey
	OnFire func()
}

// runHotkeyListener is the single-binding form, kept as a thin wrapper for
// backwards-compat. New code should use runHotkeyMultiListener.
func runHotkeyListener(ctx context.Context, h Hotkey, onFire func()) error {
	return runHotkeyMultiListener(ctx, []HotkeyBinding{{Hotkey: h, OnFire: onFire}})
}

// runHotkeyMultiListener registers N global hotkeys, then runs a single
// Windows message loop on a locked OS thread. WM_HOTKEY's wParam carries the
// registered ID (1, 2, 3...) which we use to dispatch to the right callback.
// Returns when ctx is cancelled.
func runHotkeyMultiListener(ctx context.Context, bindings []HotkeyBinding) error {
	if len(bindings) == 0 {
		<-ctx.Done()
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := procGetCurrentThreadId.Call()

	registered := make([]uintptr, 0, len(bindings))
	dispatch := make(map[uintptr]func(), len(bindings))
	for i, b := range bindings {
		if b.OnFire == nil {
			continue
		}
		id := uintptr(i + 1)
		ok, _, errno := procRegisterHotKey.Call(0, id, uintptr(b.Hotkey.Mods), uintptr(b.Hotkey.Key))
		if ok == 0 {
			// Best-effort: unregister whatever we did register, then bail.
			for _, prev := range registered {
				procUnregisterHotKey.Call(0, prev)
			}
			return fmt.Errorf("RegisterHotKey(%s): %v", b.Hotkey.String(), errno)
		}
		registered = append(registered, id)
		dispatch[id] = b.OnFire
	}
	defer func() {
		for _, id := range registered {
			procUnregisterHotKey.Call(0, id)
		}
	}()

	stopWatcher := make(chan struct{})
	defer close(stopWatcher)
	go func() {
		select {
		case <-ctx.Done():
			procPostThreadMessageW.Call(tid, uintptr(wmQuit), 0, 0)
		case <-stopWatcher:
		}
	}()

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			return nil
		}
		if m.Message != wmHotkey {
			continue
		}
		cb, ok := dispatch[m.WParam]
		if !ok {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			cb()
		}()
	}
}
