// hotkey_demo submits a long queue of click jobs and prompts the user to
// press Ctrl+Alt+Shift+S to abort. Verifies in-flight finishes its critical
// section, pending get StatusCancelled, and the listener teardown is clean.
//
// Prereq: 1 Chrome on --remote-debugging-port=9222.
package main

import (
	stdlog "log"
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
	if err := b.Current().Navigate(srv.PageURL("hk"), 30*time.Second); err != nil {
		stdlog.Fatalf("navigate: %v", err)
	}

	stopped := make(chan string, 1)
	fleet := chromefleet.New(
		chromefleet.WithLogger(stdLogger{}),
		chromefleet.WithDefaultTimeout(5*time.Second),
		chromefleet.OnStop(func(reason string) { stopped <- reason }),
	)
	if err := fleet.Register(&chromefleet.BrowserHandle{
		ID: "hk", Browser: b, X: 0, Y: 0, Scale: 1.0,
	}); err != nil {
		stdlog.Fatalf("register: %v", err)
	}

	fleet.Start()

	const total = 50
	results := make([]<-chan chromefleet.JobResult, 0, total)
	for i := 0; i < total; i++ {
		ch, err := fleet.Submit(chromefleet.Job{
			BrowserID: "hk",
			Action:    chromefleet.ClickAction{Selector: "#submit"},
			Priority:  5,
		})
		if err != nil {
			stdlog.Printf("submit %d: %v", i, err)
			continue
		}
		results = append(results, ch)
	}

	stdlog.Printf("Submitted %d jobs. Press Ctrl+Alt+Shift+S to stop.", total)

	reason := <-stopped
	stdlog.Printf("Stop fired: %s. Draining results...", reason)

	done, cancelled, failed := 0, 0, 0
	for _, ch := range results {
		r := <-ch
		switch r.Status {
		case chromefleet.StatusDone:
			done++
		case chromefleet.StatusCancelled:
			cancelled++
		case chromefleet.StatusFailed:
			failed++
		}
	}
	stdlog.Printf("Result: done=%d cancelled=%d failed=%d", done, cancelled, failed)
	stdlog.Printf("Expected: cancelled > 0 (queue was aborted mid-flight)")
	fleet.Stop()
}

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (stdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
