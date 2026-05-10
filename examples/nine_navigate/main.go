// nine_navigate connects to 9 Chrome instances on ports 9222..9230 and
// drives each through the native omnibox Navigate flow to https://example.com.
// Use it to eyeball that the new Page.Navigate behaves correctly on multiple
// browsers in sequence — you'll see the address bar of each window fill in,
// land on End, and Enter to navigate.
//
// Prereq: 9 Chrome on 9222..9230, 3x3 grid 480x270 (or any layout).
package main

import (
	stdlog "log"
	"strconv"
	"sync"
	"time"

	"github.com/tuwibu/chromekit"
)

const targetURL = "https://example.com"

func main() {
	type spec struct {
		id   string
		port int
		x, y int
	}
	var specs []spec
	const cols, cellW, cellH = 3, 480, 270
	for i := 0; i < 9; i++ {
		col := i % cols
		row := i / cols
		specs = append(specs, spec{
			id:   "b" + strconv.Itoa(i),
			port: 9222 + i,
			x:    col * cellW,
			y:    row * cellH,
		})
	}

	browsers := make([]*chromekit.Browser, 0, len(specs))
	defer func() {
		var wg sync.WaitGroup
		for _, b := range browsers {
			wg.Add(1)
			go func(b *chromekit.Browser) { defer wg.Done(); b.Close() }(b)
		}
		wg.Wait()
	}()

	for _, sp := range specs {
		b, err := chromekit.Connect(sp.port,
			chromekit.WithInputBackend(chromekit.BackendNative),
			chromekit.WithNativeWindow(sp.x, sp.y, 1.0),
			chromekit.WithLogger(stdLogger{prefix: sp.id}),
		)
		if err != nil {
			stdlog.Fatalf("connect %s: %v", sp.id, err)
		}
		browsers = append(browsers, b)
	}
	stdlog.Printf("connected %d browsers, navigating to %s sequentially", len(browsers), targetURL)

	for i, b := range browsers {
		start := time.Now()
		if err := b.Current().Navigate(targetURL, 30*time.Second); err != nil {
			stdlog.Printf("[%s] navigate fail: %v", specs[i].id, err)
			continue
		}
		got, _ := b.Current().GetURL()
		stdlog.Printf("[%s] -> %s (%v)", specs[i].id, got, time.Since(start).Round(time.Millisecond))
	}
	stdlog.Printf("done")
}

type stdLogger struct{ prefix string }

func (l stdLogger) Infof(f string, a ...any)  { stdlog.Printf("["+l.prefix+"] "+f, a...) }
func (l stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("["+l.prefix+"][warn] "+f, a...) }
func (l stdLogger) Errorf(f string, a ...any) { stdlog.Printf("["+l.prefix+"][err] "+f, a...) }
func (l stdLogger) Debugf(f string, a ...any) {}
