// two_browser launches 2 Chrome instances side-by-side and submits 20
// alternating click+type jobs, then verifies each browser's event log only
// contains its own window id (no cross-window leak).
//
// Prereq: 2 Chrome instances running with --remote-debugging-port=9222 / 9223.
//   chrome.exe --remote-debugging-port=9222 --user-data-dir=C:\tmp\chrome-9222
//   chrome.exe --remote-debugging-port=9223 --user-data-dir=C:\tmp\chrome-9223
package main

import (
	"encoding/json"
	"fmt"
	stdlog "log"
	"strconv"
	"time"

	"github.com/tuwibu/chromefleet"
	"github.com/tuwibu/chromefleet/examples/testpage"
	"github.com/tuwibu/chromekit"
)

type browserSpec struct {
	id   string
	port int
	x, y int
}

type fleetEvent struct {
	TS       int64  `json:"ts"`
	WindowID string `json:"windowId"`
	Event    string `json:"event"`
	Target   string `json:"target"`
	Value    string `json:"value"`
}

func main() {
	srv, err := testpage.Start()
	if err != nil {
		stdlog.Fatalf("start test server: %v", err)
	}
	defer srv.Close()
	stdlog.Printf("test page: %s", srv.URL)

	specs := []browserSpec{
		{"b1", 9222, 0, 0},
		{"b2", 9223, 960, 0},
	}

	fleet := chromefleet.New(
		chromefleet.WithLogger(stdLogger{}),
		chromefleet.WithDefaultTimeout(15*time.Second),
		chromefleet.WithStopHotkeyDisabled(),
	)

	browsers := make(map[string]*chromekit.Browser)

	for _, sp := range specs {
		b, err := chromekit.Connect(sp.port,
			chromekit.WithInputBackend(chromekit.BackendNative),
			chromekit.WithNativeWindow(sp.x, sp.y, 1.0),
		)
		if err != nil {
			stdlog.Fatalf("connect %s on :%d: %v", sp.id, sp.port, err)
		}
		defer b.Close()
		browsers[sp.id] = b

		if err := b.Current().Navigate(srv.PageURL(sp.id), 30*time.Second); err != nil {
			stdlog.Fatalf("navigate %s: %v", sp.id, err)
		}
		if err := fleet.Register(&chromefleet.BrowserHandle{
			ID:      sp.id,
			Browser: b,
			X:       sp.x,
			Y:       sp.y,
			Scale:   1.0,
		}); err != nil {
			stdlog.Fatalf("register %s: %v", sp.id, err)
		}
	}

	fleet.Start()
	defer fleet.Stop()

	const rounds = 10
	results := make([]<-chan chromefleet.JobResult, 0, rounds*4)
	for i := 0; i < rounds; i++ {
		for _, sp := range specs {
			ch1, err := fleet.Submit(chromefleet.Job{
				BrowserID: sp.id,
				Action: chromefleet.TypeAction{
					Selector: "#q",
					Text:     fmt.Sprintf("%s-%s ", sp.id, strconv.Itoa(i)),
				},
				Priority: 5,
			})
			if err != nil {
				stdlog.Fatalf("submit type: %v", err)
			}
			ch2, err := fleet.Submit(chromefleet.Job{
				BrowserID: sp.id,
				Action:    chromefleet.ClickAction{Selector: "#submit"},
				Priority:  5,
			})
			if err != nil {
				stdlog.Fatalf("submit click: %v", err)
			}
			results = append(results, ch1, ch2)
		}
	}

	failures := 0
	for _, ch := range results {
		r := <-ch
		if r.Status != chromefleet.StatusDone {
			failures++
			stdlog.Printf("FAIL job=%d browser=%s status=%s err=%v", r.ID, r.BrowserID, r.Status, r.Err)
		}
	}

	leaks := 0
	for _, sp := range specs {
		events, err := readFleetLog(browsers[sp.id])
		if err != nil {
			stdlog.Printf("read log %s: %v", sp.id, err)
			continue
		}
		for _, e := range events {
			if e.WindowID != sp.id {
				leaks++
				stdlog.Printf("LEAK: browser %s saw event from %s: %+v", sp.id, e.WindowID, e)
			}
		}
		stdlog.Printf("%s: %d events captured", sp.id, len(events))
	}

	if failures == 0 && leaks == 0 {
		stdlog.Printf("PASS — %d jobs, 0 failures, 0 cross-window leaks", len(results))
	} else {
		stdlog.Printf("FAIL — %d failures, %d leaks", failures, leaks)
	}
}

func readFleetLog(b *chromekit.Browser) ([]fleetEvent, error) {
	var raw string
	if err := b.Current().Evaluate(`JSON.stringify(window._fleetLog || [])`, &raw); err != nil {
		return nil, err
	}
	var out []fleetEvent
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (stdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
