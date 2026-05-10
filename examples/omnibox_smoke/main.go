// omnibox_smoke verifies that chromekit's native Navigate types into the
// Chrome address bar and dodges Chrome's inline autocomplete. Steps:
//
//  1. Connect to chrome on :9222 with native input.
//  2. CDP-navigate to <server>/?id=login to seed history.
//  3. Native-navigate (omnibox flow) to <server>/ (root).
//  4. Assert b.Current().GetURL() ends with "/" — NOT "?id=login".
//
// Without the End-key step in Page.Navigate, Chrome would inline-complete
// the typed URL with the longer history entry and Enter would re-open
// /?id=login.
//
// Prereq: chrome on :9222 with persistent --user-data-dir so history
// survives between the seed and the test (default ephemeral profile would
// not autocomplete).
package main

import (
	stdlog "log"
	"strings"
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

	root := srv.URL + "/"
	seed := srv.PageURL("login") // /?id=login

	b, err := chromekit.Connect(9222,
		chromekit.WithInputBackend(chromekit.BackendNative),
		chromekit.WithNativeWindow(0, 0, 1.0),
		chromekit.WithLogger(stdLogger{}),
	)
	if err != nil {
		stdlog.Fatalf("connect: %v", err)
	}
	defer b.Close()

	// Seed history via JS location.href so we don't go through the omnibox
	// path for the seed step itself. Chrome records the result in history
	// the same as a real navigation.
	if err := b.Current().Evaluate("window.location.href = "+jsString(seed), nil); err != nil {
		stdlog.Fatalf("seed: %v", err)
	}
	stdlog.Printf("seeded history with %s", seed)
	time.Sleep(2 * time.Second) // let chrome flush history

	// Now drive the omnibox: navigate to root. If End-key works, final URL
	// will be root; if autocomplete wins, final URL will be the seed.
	stdlog.Printf("omnibox-navigate to %s", root)
	if err := b.Current().Navigate(root, 30*time.Second); err != nil {
		stdlog.Fatalf("omnibox navigate: %v", err)
	}

	got, err := b.Current().GetURL()
	if err != nil {
		stdlog.Fatalf("getURL: %v", err)
	}
	stdlog.Printf("final URL: %s", got)
	if strings.Contains(got, "id=login") {
		stdlog.Fatalf("FAIL: autocomplete trap fired — landed on %s", got)
	}
	if !strings.HasSuffix(strings.TrimRight(got, "/"), strings.TrimRight(root, "/")) {
		stdlog.Fatalf("FAIL: expected root URL %s, got %s", root, got)
	}
	stdlog.Printf("PASS — omnibox navigation lands on root, not autocomplete suggestion")
}

// jsString quotes a Go string as a JavaScript string literal (single quotes,
// minimal escaping — fine for our localhost URLs).
func jsString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(s) + "'"
}

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (stdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
func (stdLogger) Debugf(f string, a ...any) {}
