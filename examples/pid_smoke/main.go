// pid_smoke proves PID-based HWND lookup distinguishes 2 Chrome browsers
// loading the EXACT same URL (and therefore same title). Title-based lookup
// would either return the same HWND or error with "multiple windows".
//
// Prereq: 2 Chrome instances on --remote-debugging-port=9222 and 9223.
package main

import (
	stdlog "log"
	"time"

	"github.com/tuwibu/chromefleet/examples/testpage"
	"github.com/tuwibu/chromekit"
)

func main() {
	srv, err := testpage.Start()
	if err != nil {
		stdlog.Fatalf("test page: %v", err)
	}
	defer srv.Close()

	sameURL := srv.PageURL("same") // both browsers load this -> identical title

	b1, err := chromekit.Connect(9222,
		chromekit.WithInputBackend(chromekit.BackendNative),
		chromekit.WithNativeWindow(0, 0, 1.0),
		chromekit.WithLogger(stdLogger{}),
	)
	if err != nil {
		stdlog.Fatalf("connect 9222: %v", err)
	}
	defer b1.Close()
	if err := b1.Current().Navigate(sameURL, 30*time.Second); err != nil {
		stdlog.Fatalf("nav b1: %v", err)
	}

	b2, err := chromekit.Connect(9223,
		chromekit.WithInputBackend(chromekit.BackendNative),
		chromekit.WithNativeWindow(960, 0, 1.0),
		chromekit.WithLogger(stdLogger{}),
	)
	if err != nil {
		stdlog.Fatalf("connect 9223: %v", err)
	}
	defer b2.Close()
	if err := b2.Current().Navigate(sameURL, 30*time.Second); err != nil {
		stdlog.Fatalf("nav b2: %v", err)
	}

	hwnd1, err := b1.HWND()
	if err != nil {
		stdlog.Fatalf("b1 HWND: %v", err)
	}
	hwnd2, err := b2.HWND()
	if err != nil {
		stdlog.Fatalf("b2 HWND: %v", err)
	}

	stdlog.Printf("b1 HWND = 0x%x", hwnd1)
	stdlog.Printf("b2 HWND = 0x%x", hwnd2)
	if hwnd1 == hwnd2 {
		stdlog.Fatalf("FAIL: same HWND returned for distinct browsers — PID lookup broken")
	}
	stdlog.Printf("PASS — distinct HWNDs for same-title browsers, PID lookup works")
}

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (stdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
func (stdLogger) Debugf(f string, a ...any) {}
