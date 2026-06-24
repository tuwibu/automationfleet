// mixed_fleet drives a Chrome and a Firefox instance through one fleet,
// showing the driver abstraction: chromekit and firefoxkit register into the
// same Fleet via RegisterChrome / RegisterFirefox and share the dispatcher.
//
// Chrome runs the native input path (anti-bot, single critical worker).
// Firefox runs the Remote (BiDi) path — native firefox input has a
// content-offset gap in firefoxkit, so native=false is the supported mode.
//
// Usage (requires a running Chrome on 9222 and Firefox on 9223 with remote
// debugging enabled):
//
//	go run ./examples/mixed_fleet
package main

import (
	stdlog "log"
	"time"

	"github.com/tuwibu/automationfleet"
	"github.com/tuwibu/chromekit"
	"github.com/tuwibu/firefoxkit"
)

func main() {
	fleet := automationfleet.New(
		automationfleet.WithDefaultTimeout(15 * time.Second),
		automationfleet.WithStopHotkeyDisabled(),
	)
	fleet.Start()
	defer fleet.Stop()

	// Chrome — native input path.
	chrome, err := chromekit.Connect(9222,
		chromekit.WithInputBackend(chromekit.BackendNative),
		chromekit.WithNativeWindow(0, 0, 1.0),
	)
	if err != nil {
		stdlog.Fatalf("chrome connect: %v", err)
	}
	defer chrome.Close()
	if err := fleet.RegisterChrome("chrome-1", chrome, true, 0, 0, 1.0); err != nil {
		stdlog.Fatalf("register chrome: %v", err)
	}

	// Firefox — Remote (BiDi) path. native=false is the supported mode.
	firefox, err := firefoxkit.Connect(9223)
	if err != nil {
		stdlog.Fatalf("firefox connect: %v", err)
	}
	defer firefox.Close()
	if err := fleet.RegisterFirefox("firefox-1", firefox, false, 0, 0, 1.0); err != nil {
		stdlog.Fatalf("register firefox: %v", err)
	}

	// Submit the same script to both browsers; the fleet routes chrome through
	// the native worker and firefox through the parallel CDP/BiDi pool.
	for _, id := range []string{"chrome-1", "firefox-1"} {
		nav, _ := fleet.Submit(automationfleet.Job{
			BrowserID: id,
			Action:    automationfleet.NavigateAction{URL: "https://example.com"},
			Priority:  5,
		})
		if r := <-nav; r.Err != nil {
			stdlog.Printf("%s navigate: %v", id, r.Err)
			continue
		}
		typed, _ := fleet.Submit(automationfleet.Job{
			BrowserID: id,
			Action:    automationfleet.TypeAction{Selector: "input", Text: "hello", ClearFirst: true},
			Priority:  5,
		})
		r := <-typed
		stdlog.Printf("%s type: status=%s err=%v took=%s", id, r.Status, r.Err, r.Took)
	}

	fleet.Wait()
	stdlog.Printf("mixed fleet done — chrome (native) + firefox (BiDi) driven through one Fleet")
}
