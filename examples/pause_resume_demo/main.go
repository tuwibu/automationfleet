// pause_resume_demo submits 100 click jobs against 1 Chrome and demonstrates
// the global pause / resume hotkeys. Press Ctrl+F10 mid-run to freeze the
// queue, Ctrl+F11 to continue. In-flight jobs always finish their critical
// section even when pause hits.
//
// Prereq: 1 Chrome on --remote-debugging-port=9222.
package main

import (
	stdlog "log"
	"sync/atomic"
	"time"

	"github.com/tuwibu/chromefleet"
	"github.com/tuwibu/chromefleet/examples/testpage"
	"github.com/tuwibu/chromekit"
)

func main() {
	srv, err := testpage.Start()
	if err != nil {
		stdlog.Fatalf("test page: %v", err)
	}
	defer srv.Close()

	b, err := chromekit.Connect(9222,
		chromekit.WithInputBackend(chromekit.BackendNative),
		chromekit.WithNativeWindow(0, 0, 1.0),
	)
	if err != nil {
		stdlog.Fatalf("connect: %v", err)
	}
	defer b.Close()
	if err := b.Current().Navigate(srv.PageURL("pr"), 30*time.Second); err != nil {
		stdlog.Fatalf("navigate: %v", err)
	}

	var done int32
	fleet := chromefleet.New(
		chromefleet.WithLogger(stdLogger{}),
		chromefleet.WithDefaultTimeout(5*time.Second),
		chromefleet.OnPause(func(reason string) {
			stdlog.Printf(">>> PAUSED at done=%d (%s)", atomic.LoadInt32(&done), reason)
		}),
		chromefleet.OnResume(func(reason string) {
			stdlog.Printf(">>> RESUMED at done=%d (%s)", atomic.LoadInt32(&done), reason)
		}),
	)
	if err := fleet.Register(&chromefleet.BrowserHandle{
		ID: "pr", Browser: b, X: 0, Y: 0, Scale: 1.0,
	}); err != nil {
		stdlog.Fatalf("register: %v", err)
	}
	fleet.Start()
	defer fleet.Stop()

	const total = 100
	results := make([]<-chan chromefleet.JobResult, 0, total)
	for i := 0; i < total; i++ {
		ch, err := fleet.Submit(chromefleet.Job{
			BrowserID: "pr",
			Action:    chromefleet.ClickAction{Selector: "#submit"},
			Priority:  5,
		})
		if err != nil {
			stdlog.Printf("submit %d: %v", i, err)
			continue
		}
		results = append(results, ch)
	}
	stdlog.Printf("Submitted %d jobs. Press Ctrl+F10 to pause, Ctrl+F11 to resume.", total)

	failed, cancelled := 0, 0
	for _, ch := range results {
		r := <-ch
		switch r.Status {
		case chromefleet.StatusDone:
			atomic.AddInt32(&done, 1)
		case chromefleet.StatusCancelled:
			cancelled++
		case chromefleet.StatusFailed:
			failed++
		}
	}
	stdlog.Printf("Result: done=%d cancelled=%d failed=%d", atomic.LoadInt32(&done), cancelled, failed)
}

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (stdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
