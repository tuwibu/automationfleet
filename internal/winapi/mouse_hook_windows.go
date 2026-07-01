//go:build windows

package winapi

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId  = kernel32.NewProc("GetCurrentThreadId")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

const (
	whMouseLL = 14     // WH_MOUSE_LL
	wmQuit    = 0x0012 // WM_QUIT
)

// mslLHookStruct is the Win32 MSLLHOOKSTRUCT passed to a WH_MOUSE_LL proc.
type mslLHookStruct struct {
	Pt          point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// winMsg mirrors the Win32 MSG struct for the message pump.
type winMsg struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	_       uint32
}

// realMoveCallback is invoked from the hook thread on every genuine hardware
// mouse event. Guarded by realMoveMu because install/uninstall re-sets it; the
// hook only ever runs while a callback is installed. The lock is uncontended
// (set twice per install lifecycle) so it stays far under LowLevelHooksTimeout.
var (
	realMoveMu       sync.Mutex
	realMoveCallback func()

	// hookInstalled enforces the single-hook invariant at the package boundary:
	// realMoveCallback is a process-global, so a second concurrent install would
	// silently clobber the first. Callers (fleet.go) already serialize via their
	// own lock, but this guard fails loud instead of corrupting silently if that
	// discipline is ever broken.
	hookInstalled atomic.Bool
)

func setRealMoveCallback(cb func()) {
	realMoveMu.Lock()
	realMoveCallback = cb
	realMoveMu.Unlock()
}

// mouseProcCb is the WH_MOUSE_LL hook procedure. It MUST stay minimal —
// Windows silently drops the hook if this callback exceeds
// LowLevelHooksTimeout (~300ms). It classifies the event, fires a
// non-blocking callback for real hardware input, then ALWAYS chains to the
// next hook via CallNextHookEx.
var mouseProcCb = syscall.NewCallback(func(nCode uintptr, wParam uintptr, lParam uintptr) uintptr {
	if int32(nCode) >= 0 {
		// lParam is an OS-owned pointer handed to the hook proc as a uintptr; it
		// points into Win32 memory (never the Go heap) and stays valid for the
		// callback's duration, so the uintptr→Pointer conversion is safe. go vet's
		// unsafeptr analyzer flags this as a false positive — run with
		// `-unsafeptr=false` for a clean pass.
		s := (*mslLHookStruct)(unsafe.Pointer(lParam))
		if IsRealMouseMove(s.Flags) {
			realMoveMu.Lock()
			cb := realMoveCallback
			realMoveMu.Unlock()
			if cb != nil {
				cb()
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
	return ret
})

// InstallMouseHook installs a global WH_MOUSE_LL hook and invokes onRealMove
// (from the hook thread, non-blocking expected) for every genuine hardware
// mouse event — SendInput-injected events are filtered out via LLMHF_INJECTED.
// A dedicated locked-OS-thread goroutine owns the required message pump (the
// low-level hook only fires while that thread pumps GetMessage). The returned
// func uninstalls the hook and tears down the pump; it blocks until the thread
// exits. Returns an error if SetWindowsHookEx fails.
func InstallMouseHook(onRealMove func()) (func(), error) {
	if !hookInstalled.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("winapi: mouse hook already installed")
	}
	type installResult struct {
		tid uintptr
		err error
	}
	ready := make(chan installResult, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		setRealMoveCallback(onRealMove)
		defer setRealMoveCallback(nil)

		tid, _, _ := procGetCurrentThreadId.Call()
		hMod, _, _ := procGetModuleHandleW.Call(0)
		hHook, _, errno := procSetWindowsHookExW.Call(uintptr(whMouseLL), mouseProcCb, hMod, 0)
		if hHook == 0 {
			ready <- installResult{err: fmt.Errorf("winapi: SetWindowsHookEx(WH_MOUSE_LL): %v", errno)}
			return
		}
		defer procUnhookWindowsHookEx.Call(hHook)
		ready <- installResult{tid: tid}

		var m winMsg
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			// GetMessage returns 0 on WM_QUIT, -1 on error → stop the pump.
			if int32(ret) <= 0 {
				return
			}
		}
	}()

	r := <-ready
	if r.err != nil {
		<-done
		hookInstalled.Store(false)
		return nil, r.err
	}
	uninstall := func() {
		procPostThreadMessageW.Call(r.tid, uintptr(wmQuit), 0, 0)
		<-done
		hookInstalled.Store(false)
	}
	return uninstall, nil
}
