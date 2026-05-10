// stress_nine launches up to 9 Chrome instances in a 3x3 grid and runs N
// random jobs across them. Prints latency p50/p95/p99 + error counts.
//
// Prereq: 9 Chrome instances on ports 9222..9230, each with a separate
// --user-data-dir, all visible (not minimized).
package main

import (
	"flag"
	stdlog "log"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/tuwibu/chromefleet"
	"github.com/tuwibu/chromefleet/examples/testpage"
	"github.com/tuwibu/chromekit"
)

func main() {
	jobs := flag.Int("jobs", 100, "total jobs to submit")
	browsers := flag.Int("browsers", 9, "number of browsers")
	cellW := flag.Int("cell-w", 640, "grid cell width px")
	cellH := flag.Int("cell-h", 360, "grid cell height px")
	flag.Parse()

	srv, err := testpage.Start()
	if err != nil {
		stdlog.Fatalf("test page: %v", err)
	}
	defer srv.Close()

	fleet := chromefleet.New(
		chromefleet.WithLogger(stdLogger{}),
		chromefleet.WithDefaultTimeout(15*time.Second),
		chromefleet.WithStopHotkeyDisabled(),
	)
	defer fleet.Stop()

	type spec struct {
		id   string
		port int
		x, y int
	}
	var specs []spec
	for i := 0; i < *browsers; i++ {
		col := i % 3
		row := i / 3
		specs = append(specs, spec{
			id:   "b" + strconv.Itoa(i),
			port: 9222 + i,
			x:    col * *cellW,
			y:    row * *cellH,
		})
	}

	for _, sp := range specs {
		b, err := chromekit.Connect(sp.port,
			chromekit.WithInputBackend(chromekit.BackendNative),
			chromekit.WithNativeWindow(sp.x, sp.y, 1.0),
		)
		if err != nil {
			stdlog.Fatalf("connect %s: %v", sp.id, err)
		}
		defer b.Close()
		if err := b.Current().Navigate(srv.PageURL(sp.id), 30*time.Second); err != nil {
			stdlog.Fatalf("navigate %s: %v", sp.id, err)
		}
		if err := fleet.Register(&chromefleet.BrowserHandle{
			ID: sp.id, Browser: b, X: sp.x, Y: sp.y, Scale: 1.0,
		}); err != nil {
			stdlog.Fatalf("register %s: %v", sp.id, err)
		}
	}

	fleet.Start()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	results := make([]<-chan chromefleet.JobResult, *jobs)
	starts := make([]time.Time, *jobs)
	for i := 0; i < *jobs; i++ {
		sp := specs[rng.Intn(len(specs))]
		var action chromefleet.Action
		if rng.Intn(2) == 0 {
			action = chromefleet.ClickAction{Selector: "#submit"}
		} else {
			action = chromefleet.TypeAction{
				Selector: "#q",
				Text:     "j" + strconv.Itoa(i) + " ",
			}
		}
		starts[i] = time.Now()
		ch, err := fleet.Submit(chromefleet.Job{
			BrowserID: sp.id,
			Action:    action,
			Priority:  rng.Intn(10),
		})
		if err != nil {
			stdlog.Fatalf("submit %d: %v", i, err)
		}
		results[i] = ch
	}

	var (
		mu        sync.Mutex
		latencies []time.Duration
		done, fail, cancel int
	)
	var wg sync.WaitGroup
	for i, ch := range results {
		wg.Add(1)
		go func(i int, ch <-chan chromefleet.JobResult) {
			defer wg.Done()
			r := <-ch
			mu.Lock()
			defer mu.Unlock()
			latencies = append(latencies, r.Took)
			switch r.Status {
			case chromefleet.StatusDone:
				done++
			case chromefleet.StatusCancelled:
				cancel++
			default:
				fail++
				stdlog.Printf("FAIL job=%d status=%s err=%v", r.ID, r.Status, r.Err)
			}
		}(i, ch)
	}
	wg.Wait()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)-1) * p)
		return latencies[idx]
	}
	stdlog.Printf("=== stress_nine result ===")
	stdlog.Printf("jobs=%d done=%d fail=%d cancel=%d", *jobs, done, fail, cancel)
	stdlog.Printf("latency p50=%s p95=%s p99=%s max=%s",
		pct(0.5), pct(0.95), pct(0.99), pct(1.0))
}

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (stdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
