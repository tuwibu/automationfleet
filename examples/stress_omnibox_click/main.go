// stress_omnibox_click hammers N Chrome instances with a randomised mix of
// omnibox NavigateActions, ClickActions and TypeActions — all routed through
// chromefleet's native critical section.
//
// Three phases run in order:
//
//   1. Spawn / connect to N Chrome instances and seed each with the test page.
//   2. SMOKE: per browser, submit one Navigate + one Click + one Type and
//      assert the page state actually changed (URL, click counter, event log).
//      Catches "input went into the wrong window" before the stress phase
//      buries the signal under thousands of jobs.
//   3. STRESS: submit `-jobs` randomised jobs, report p50/p95/p99 latency by
//      action kind and aggregate.
//
// Two Chrome-acquisition modes:
//
//   * Default — Connect to existing Chrome on ports 9222..9222+N-1.
//   * `-chrome-path <chrome.exe>` — spawn our own Chrome processes via
//     os/exec with explicit `--remote-debugging-port`s, then Connect normally.
//     Avoids chromekit.Launch which (currently) doesn't populate the PID and
//     therefore breaks native HWND lookup on a fresh about:blank.
//
// Usage:
//
//	go run ./examples/stress_omnibox_click \
//	    -browsers 9 -jobs 200 -nav-ratio 0.3 -click-ratio 0.4 \
//	    -chrome-path "C:\Program Files\Google\Chrome\Application\chrome.exe"
//
// nav-ratio + click-ratio + (1 - both) = type ratio. Defaults give a healthy
// mix that keeps the omnibox active without thrashing it.
package main

import (
	"flag"
	"fmt"
	stdlog "log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tuwibu/chromefleet"
	"github.com/tuwibu/chromefleet/examples/testpage"
	"github.com/tuwibu/chromekit"
)

func main() {
	jobs := flag.Int("jobs", 120, "total jobs to submit in the stress phase")
	browsers := flag.Int("browsers", 4, "number of browsers (ports 9222..9222+N-1)")
	cellW := flag.Int("cell-w", 640, "grid cell width px")
	cellH := flag.Int("cell-h", 360, "grid cell height px")
	cols := flag.Int("cols", 3, "grid columns")
	navRatio := flag.Float64("nav-ratio", 0.3, "fraction of stress jobs that are NavigateAction")
	clickRatio := flag.Float64("click-ratio", 0.4, "fraction of stress jobs that are ClickAction")
	timeoutSec := flag.Int("timeout", 20, "per-job timeout (seconds)")
	seed := flag.Int64("seed", 0, "RNG seed (0 = time-based)")
	chromePath := flag.String("chrome-path", "", "path to chrome.exe — when set, the test spawns its own Chrome processes with explicit --remote-debugging-port. Each instance gets a temp profile under %TEMP%/fleet-stress-<pid>/bN.")
	skipSmoke := flag.Bool("skip-smoke", false, "skip the smoke phase (assertions) and go straight to stress")
	flag.Parse()

	if *navRatio < 0 || *clickRatio < 0 || *navRatio+*clickRatio > 1.0 {
		stdlog.Fatalf("invalid ratios: nav=%.2f click=%.2f (sum must be ≤ 1.0)", *navRatio, *clickRatio)
	}

	srv, err := testpage.Start()
	if err != nil {
		stdlog.Fatalf("test page: %v", err)
	}
	defer srv.Close()

	type spec struct {
		id   string
		port int
		x, y int
	}
	specs := make([]spec, 0, *browsers)
	for i := 0; i < *browsers; i++ {
		col := i % *cols
		row := i / *cols
		specs = append(specs, spec{
			id:   "b" + strconv.Itoa(i),
			port: 9222 + i,
			x:    col * *cellW,
			y:    row * *cellH,
		})
	}

	var spawned []*exec.Cmd
	defer func() {
		for _, c := range spawned {
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		}
	}()

	profileRoot := filepath.Join(os.TempDir(), fmt.Sprintf("fleet-stress-%d", os.Getpid()))
	if *chromePath != "" {
		_ = os.MkdirAll(profileRoot, 0o755)
		stdlog.Printf("spawn mode: chrome=%s profile-root=%s", *chromePath, profileRoot)
		for _, sp := range specs {
			profile := filepath.Join(profileRoot, sp.id)
			cmd, err := spawnChrome(*chromePath, sp.port, profile, sp.x, sp.y, *cellW, *cellH)
			if err != nil {
				stdlog.Fatalf("spawn %s: %v", sp.id, err)
			}
			spawned = append(spawned, cmd)
		}
		// Wait for every CDP endpoint to come up. Chrome takes ~300-800ms
		// to bind --remote-debugging-port; polling beats a fixed sleep.
		for _, sp := range specs {
			if err := waitDevtools(sp.port, 15*time.Second); err != nil {
				stdlog.Fatalf("wait devtools %s: %v", sp.id, err)
			}
		}
	}

	fleet := chromefleet.New(
		chromefleet.WithLogger(stdLogger{}),
		chromefleet.WithDefaultTimeout(time.Duration(*timeoutSec)*time.Second),
		chromefleet.WithStopHotkeyDisabled(),
	)
	defer fleet.Stop()

	browserByID := make(map[string]*chromekit.Browser, len(specs))
	for _, sp := range specs {
		b, err := chromekit.Connect(sp.port,
			chromekit.WithInputBackend(chromekit.BackendNative),
			chromekit.WithNativeWindow(sp.x, sp.y, 1.0),
		)
		if err != nil {
			stdlog.Fatalf("connect %s: %v", sp.id, err)
		}
		defer b.Close()
		browserByID[sp.id] = b

		// CDP-navigate to the test page. Connect path lets chromekit
		// resolve PID via the port, so HWND works regardless of title.
		if err := b.Current().Navigate(srv.PageURL(sp.id), 30*time.Second); err != nil {
			// Fallback: BackendNative routes Navigate through the
			// omnibox which needs HWND. If the spawned Chrome opens
			// to about:blank with no title the very first Navigate
			// can race. Seed via JS instead.
			seedJS := "window.location.href = " + jsString(srv.PageURL(sp.id))
			if err2 := b.Current().Evaluate(seedJS, nil); err2 != nil {
				stdlog.Fatalf("seed %s: %v (after navigate err: %v)", sp.id, err2, err)
			}
			if err2 := b.Current().WaitForSelector("#submit", 30*time.Second); err2 != nil {
				stdlog.Fatalf("seed wait %s: %v", sp.id, err2)
			}
		}
		if err := fleet.Register(&chromefleet.BrowserHandle{
			ID: sp.id, Browser: b, X: sp.x, Y: sp.y, Scale: 1.0,
		}); err != nil {
			stdlog.Fatalf("register %s: %v", sp.id, err)
		}
	}

	fleet.Start()

	// ---------- Phase 2: smoke (assertions) ----------
	if !*skipSmoke {
		stdlog.Printf("=== smoke phase: 1× Navigate + 1× Click + 1× Type per browser ===")
		smokeFailed := 0
		for _, sp := range specs {
			if err := smokeOne(fleet, browserByID[sp.id], sp.id, srv.URL); err != nil {
				stdlog.Printf("SMOKE FAIL %s: %v", sp.id, err)
				smokeFailed++
				continue
			}
			stdlog.Printf("SMOKE PASS %s", sp.id)
		}
		if smokeFailed > 0 {
			stdlog.Fatalf("smoke phase: %d/%d browsers failed — aborting before stress", smokeFailed, len(specs))
		}
	}

	// ---------- Phase 3: stress ----------
	rngSeed := *seed
	if rngSeed == 0 {
		rngSeed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(rngSeed))
	stdlog.Printf("=== stress phase: seed=%d browsers=%d jobs=%d nav=%.2f click=%.2f type=%.2f ===",
		rngSeed, *browsers, *jobs, *navRatio, *clickRatio, 1-*navRatio-*clickRatio)

	navTargets := []string{
		srv.URL + "/?id=login",
		srv.URL + "/?id=home",
		srv.URL + "/?id=profile",
		srv.URL + "/?id=settings",
		srv.URL + "/",
	}

	type submission struct {
		idx       int
		kind      string
		browserID string
		ch        <-chan chromefleet.JobResult
	}
	subs := make([]submission, 0, *jobs)

	for i := 0; i < *jobs; i++ {
		sp := specs[rng.Intn(len(specs))]
		roll := rng.Float64()
		var (
			action chromefleet.Action
			kind   string
		)
		switch {
		case roll < *navRatio:
			action = chromefleet.NavigateAction{
				URL:     navTargets[rng.Intn(len(navTargets))],
				Timeout: time.Duration(*timeoutSec) * time.Second,
			}
			kind = "navigate"
		case roll < *navRatio+*clickRatio:
			action = chromefleet.ClickAction{Selector: "#submit"}
			kind = "click"
		default:
			action = chromefleet.TypeAction{
				Selector: "#q",
				Text:     "j" + strconv.Itoa(i) + " ",
			}
			kind = "type"
		}
		ch, err := fleet.Submit(chromefleet.Job{
			BrowserID: sp.id,
			Action:    action,
			Priority:  rng.Intn(10),
		})
		if err != nil {
			stdlog.Fatalf("submit %d: %v", i, err)
		}
		subs = append(subs, submission{idx: i, kind: kind, browserID: sp.id, ch: ch})
	}

	type bucket struct {
		latencies          []time.Duration
		done, fail, cancel int
	}
	stats := map[string]*bucket{
		"navigate": {}, "click": {}, "type": {},
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, s := range subs {
		wg.Add(1)
		go func(s submission) {
			defer wg.Done()
			r := <-s.ch
			mu.Lock()
			defer mu.Unlock()
			b := stats[s.kind]
			b.latencies = append(b.latencies, r.Took)
			switch r.Status {
			case chromefleet.StatusDone:
				b.done++
			case chromefleet.StatusCancelled:
				b.cancel++
			default:
				b.fail++
				stdlog.Printf("FAIL job=%d kind=%s browser=%s status=%s err=%v",
					r.ID, s.kind, s.browserID, r.Status, r.Err)
			}
		}(s)
	}
	wg.Wait()

	stdlog.Printf("=== stress result ===")
	order := []string{"navigate", "click", "type"}
	var grandTotal []time.Duration
	for _, k := range order {
		b := stats[k]
		if len(b.latencies) == 0 {
			stdlog.Printf("[%s] no jobs", k)
			continue
		}
		sort.Slice(b.latencies, func(i, j int) bool { return b.latencies[i] < b.latencies[j] })
		stdlog.Printf("[%s] n=%d done=%d fail=%d cancel=%d  p50=%s p95=%s p99=%s max=%s",
			k, len(b.latencies), b.done, b.fail, b.cancel,
			pct(b.latencies, 0.50), pct(b.latencies, 0.95),
			pct(b.latencies, 0.99), pct(b.latencies, 1.00))
		grandTotal = append(grandTotal, b.latencies...)
	}
	if len(grandTotal) > 0 {
		sort.Slice(grandTotal, func(i, j int) bool { return grandTotal[i] < grandTotal[j] })
		stdlog.Printf("[ALL ] n=%d  p50=%s p95=%s p99=%s max=%s",
			len(grandTotal),
			pct(grandTotal, 0.50), pct(grandTotal, 0.95),
			pct(grandTotal, 0.99), pct(grandTotal, 1.00))
	}
}

// smokeOne runs Navigate → Click → Type sequentially through the fleet on a
// single browser and asserts the page state actually changed. Returns nil on
// pass, error describing the mismatch on fail.
func smokeOne(fleet *chromefleet.Fleet, b *chromekit.Browser, id, baseURL string) error {
	target := baseURL + "/?id=" + id + "-smoke"

	// Navigate via omnibox.
	if err := submitWait(fleet, id, chromefleet.NavigateAction{URL: target, Timeout: 15 * time.Second}); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	gotURL, err := b.Current().GetURL()
	if err != nil {
		return fmt.Errorf("getURL after navigate: %w", err)
	}
	if !strings.Contains(gotURL, "id="+id+"-smoke") {
		return fmt.Errorf("URL mismatch: want id=%s-smoke, got %s", id, gotURL)
	}
	// Page DOM may need a tick to settle after omnibox Enter.
	if err := b.Current().WaitForSelector("#q", 10*time.Second); err != nil {
		return fmt.Errorf("wait #q: %w", err)
	}

	// Type into the input.
	const typed = "smoke-text-123"
	if err := submitWait(fleet, id, chromefleet.TypeAction{Selector: "#q", Text: typed}); err != nil {
		return fmt.Errorf("type: %w", err)
	}
	var qVal string
	if err := b.Current().Evaluate(`document.getElementById('q').value`, &qVal); err != nil {
		return fmt.Errorf("read #q: %w", err)
	}
	if qVal != typed {
		return fmt.Errorf("input value mismatch: want %q, got %q", typed, qVal)
	}

	// Click the submit button — counter goes 0→1 and event log appends.
	if err := submitWait(fleet, id, chromefleet.ClickAction{Selector: "#submit"}); err != nil {
		return fmt.Errorf("click: %w", err)
	}
	var counter int
	if err := b.Current().Evaluate(`parseInt(document.getElementById('counter').textContent, 10)`, &counter); err != nil {
		return fmt.Errorf("read counter: %w", err)
	}
	if counter != 1 {
		return fmt.Errorf("counter mismatch: want 1, got %d", counter)
	}
	var lastClickValue string
	if err := b.Current().Evaluate(
		`(window._fleetLog.filter(e => e.event === 'click').slice(-1)[0] || {}).value || ''`,
		&lastClickValue,
	); err != nil {
		return fmt.Errorf("read fleetLog: %w", err)
	}
	if lastClickValue != typed {
		return fmt.Errorf("click event value mismatch: want %q, got %q (input may have leaked to wrong window)", typed, lastClickValue)
	}
	return nil
}

func submitWait(fleet *chromefleet.Fleet, id string, action chromefleet.Action) error {
	ch, err := fleet.Submit(chromefleet.Job{BrowserID: id, Action: action, Priority: 5})
	if err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	r := <-ch
	if r.Status != chromefleet.StatusDone {
		return fmt.Errorf("status=%s err=%v", r.Status, r.Err)
	}
	return nil
}

// spawnChrome starts a Chrome process bound to the given remote-debugging
// port, isolated to its own --user-data-dir, positioned at (x,y) and sized
// (w,h). Returns the *exec.Cmd so the caller can kill it on teardown.
func spawnChrome(chromePath string, port int, profile string, x, y, w, h int) (*exec.Cmd, error) {
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return nil, err
	}
	args := []string{
		"--remote-debugging-port=" + strconv.Itoa(port),
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-features=Translate,OptimizationHints",
		fmt.Sprintf("--window-position=%d,%d", x, y),
		fmt.Sprintf("--window-size=%d,%d", w, h),
		"about:blank",
	}
	cmd := exec.Command(chromePath, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// waitDevtools polls the Chrome DevTools endpoint until it answers /json/version
// or the deadline expires. ~50ms cadence keeps startup latency low.
func waitDevtools(port int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("devtools on %s did not become reachable within %s", addr, timeout)
}

func jsString(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == '\'' {
			out = append(out, '\\')
		}
		out = append(out, c)
	}
	out = append(out, '\'')
	return string(out)
}

func pct(xs []time.Duration, p float64) time.Duration {
	if len(xs) == 0 {
		return 0
	}
	idx := int(float64(len(xs)-1) * p)
	return xs[idx]
}

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (stdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
