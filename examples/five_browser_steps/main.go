// five_browser_steps drives 5 Chrome instances in parallel through the fleet
// queue. Per browser i (1..5) the script runs:
//
//  1. NavigateAction → testpage/?id=step1-{i}     (omnibox)
//  2. NavigateAction → testpage/?id=step2-{i}     (omnibox)
//  3. TypeAction "stepN-first" with ClearFirst=true   (round 1)
//  4. TypeAction "stepN-final" with ClearFirst=true   (round 2 replaces round 1)
//  5. ClickAction #submit
//
// Round 2 is the regression check on ClearFirst: if Ctrl+A + Delete misfires,
// round 2's text gets appended to round 1's instead of replacing it.
//
// Verification per browser:
//   - URL ends with ?id=step2-{i}
//   - #q.value == "stepN-final"   (round 1 was wiped)
//   - counter == 1
//   - last fleetLog click event value == "stepN-final"
//
// Two Chrome-acquisition modes (mirrors stress_omnibox_click):
//   - Default: Connect to existing Chrome on 9222..9226.
//   - -chrome-path <chrome.exe>: spawn our own Chrome processes, each on its
//     own --remote-debugging-port + temp profile, then Connect.
//
// Usage:
//
//	go run ./examples/five_browser_steps \
//	    -chrome-path "C:\Program Files\Google\Chrome\Application\chrome.exe"
package main

import (
	"flag"
	"fmt"
	stdlog "log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tuwibu/chromefleet"
	"github.com/tuwibu/chromefleet/examples/testpage"
	"github.com/tuwibu/chromekit"
)

const (
	browsers = 5
	cellW    = 640
	cellH    = 360
	cols     = 3
)

func main() {
	chromePath := flag.String("chrome-path", "", "path to chrome.exe — spawn own Chrome instances; if empty, Connect to running Chrome on 9222..9226")
	timeoutSec := flag.Int("timeout", 20, "per-job timeout (seconds)")
	flag.Parse()

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
	specs := make([]spec, 0, browsers)
	for i := 0; i < browsers; i++ {
		specs = append(specs, spec{
			id:   "b" + strconv.Itoa(i+1),
			port: 9222 + i,
			x:    (i % cols) * cellW,
			y:    (i / cols) * cellH,
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

	if *chromePath != "" {
		profileRoot := filepath.Join(os.TempDir(), fmt.Sprintf("fleet-5steps-%d", os.Getpid()))
		_ = os.MkdirAll(profileRoot, 0o755)
		stdlog.Printf("spawn mode: chrome=%s profile-root=%s", *chromePath, profileRoot)
		for _, sp := range specs {
			profile := filepath.Join(profileRoot, sp.id)
			cmd, err := spawnChrome(*chromePath, sp.port, profile, sp.x, sp.y, cellW, cellH)
			if err != nil {
				stdlog.Fatalf("spawn %s: %v", sp.id, err)
			}
			spawned = append(spawned, cmd)
		}
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

		// Seed via JS — Connect path with about:blank can race the first
		// omnibox Navigate. Same pattern as stress_omnibox_click.
		if err := b.Current().Evaluate("window.location.href = "+jsString(srv.PageURL("seed")), nil); err != nil {
			stdlog.Fatalf("seed %s: %v", sp.id, err)
		}
		if err := b.Current().WaitForSelector("#submit", 30*time.Second); err != nil {
			stdlog.Fatalf("seed wait %s: %v", sp.id, err)
		}

		if err := fleet.Register(&chromefleet.BrowserHandle{
			ID: sp.id, Browser: b, X: sp.x, Y: sp.y, Scale: 1.0,
		}); err != nil {
			stdlog.Fatalf("register %s: %v", sp.id, err)
		}
	}

	fleet.Start()

	// Build the per-browser script. All browsers run concurrently as far as
	// the queue is concerned; the dispatcher's single native worker
	// serialises the actual keystrokes.
	stdlog.Printf("=== running 5-browser script ===")
	var wg sync.WaitGroup
	results := make([]error, len(specs))
	for i, sp := range specs {
		wg.Add(1)
		go func(idx int, sp spec) {
			defer wg.Done()
			stt := idx + 1
			round1 := fmt.Sprintf("step%d-first", stt)
			round2 := fmt.Sprintf("step%d-final", stt)
			results[idx] = runScript(fleet, browserByID[sp.id], sp.id, srv, stt, round1, round2, time.Duration(*timeoutSec)*time.Second)
		}(i, sp)
	}
	wg.Wait()

	failed := 0
	for i, sp := range specs {
		if err := results[i]; err != nil {
			stdlog.Printf("FAIL %s: %v", sp.id, err)
			failed++
		} else {
			stdlog.Printf("PASS %s", sp.id)
		}
	}
	if failed > 0 {
		stdlog.Fatalf("%d/%d browsers failed", failed, len(specs))
	}
	stdlog.Printf("ALL PASS — 5 browsers each ran step1 → step2 → type → click")
}

// runScript drives one browser through the 5-action sequence and verifies
// the final page state. round1 is typed first then replaced by round2 via
// ClearFirst — the second TypeAction's ClearFirst is the regression check.
func runScript(fleet *chromefleet.Fleet, b *chromekit.Browser, id string, srv *testpage.Server, stt int, round1, round2 string, timeout time.Duration) error {
	step1ID := fmt.Sprintf("step1-%d", stt)
	step2ID := fmt.Sprintf("step2-%d", stt)

	// 1) Navigate ?id=step1-{stt}
	if err := submitWait(fleet, id, chromefleet.NavigateAction{URL: srv.PageURL(step1ID), Timeout: timeout}); err != nil {
		return fmt.Errorf("nav %s: %w", step1ID, err)
	}
	// 2) Navigate ?id=step2-{stt}
	if err := submitWait(fleet, id, chromefleet.NavigateAction{URL: srv.PageURL(step2ID), Timeout: timeout}); err != nil {
		return fmt.Errorf("nav %s: %w", step2ID, err)
	}
	if err := b.Current().WaitForSelector("#q", 10*time.Second); err != nil {
		return fmt.Errorf("wait #q after %s: %w", step2ID, err)
	}
	// 3) Round 1: type round1 (clear-first defensive — field should be empty).
	if err := submitWait(fleet, id, chromefleet.TypeAction{Selector: "#q", Text: round1, ClearFirst: true}); err != nil {
		return fmt.Errorf("type round1 %q: %w", round1, err)
	}
	// 3a) Sanity: round1 actually landed in the field.
	var qAfterR1 string
	if err := b.Current().Evaluate(`document.getElementById('q').value`, &qAfterR1); err != nil {
		return fmt.Errorf("read #q after round1: %w", err)
	}
	if qAfterR1 != round1 {
		return fmt.Errorf("round1 mismatch: want %q, got %q", round1, qAfterR1)
	}
	// 4) Round 2: type round2 with ClearFirst — must REPLACE round1, not append.
	if err := submitWait(fleet, id, chromefleet.TypeAction{Selector: "#q", Text: round2, ClearFirst: true}); err != nil {
		return fmt.Errorf("type round2 %q: %w", round2, err)
	}
	// 5) Click submit
	if err := submitWait(fleet, id, chromefleet.ClickAction{Selector: "#submit"}); err != nil {
		return fmt.Errorf("click submit: %w", err)
	}

	// Verify final state.
	gotURL, err := b.Current().GetURL()
	if err != nil {
		return fmt.Errorf("getURL: %w", err)
	}
	if !strings.Contains(gotURL, "id="+step2ID) {
		return fmt.Errorf("URL want id=%s, got %s", step2ID, gotURL)
	}
	var qVal string
	if err := b.Current().Evaluate(`document.getElementById('q').value`, &qVal); err != nil {
		return fmt.Errorf("read #q: %w", err)
	}
	if qVal != round2 {
		return fmt.Errorf("input value want %q, got %q (ClearFirst on round2 may have failed — round1 prefix?)", round2, qVal)
	}
	var counter int
	if err := b.Current().Evaluate(`parseInt(document.getElementById('counter').textContent, 10)`, &counter); err != nil {
		return fmt.Errorf("read counter: %w", err)
	}
	if counter != 1 {
		return fmt.Errorf("counter want 1, got %d", counter)
	}
	var lastClickValue string
	if err := b.Current().Evaluate(
		`(window._fleetLog.filter(e => e.event === 'click').slice(-1)[0] || {}).value || ''`,
		&lastClickValue,
	); err != nil {
		return fmt.Errorf("read fleetLog: %w", err)
	}
	if lastClickValue != round2 {
		return fmt.Errorf("click event value want %q, got %q (input may have leaked to wrong window)", round2, lastClickValue)
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
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(s) + "'"
}

type stdLogger struct{}

func (stdLogger) Infof(f string, a ...any)  { stdlog.Printf("[info] "+f, a...) }
func (stdLogger) Warnf(f string, a ...any)  { stdlog.Printf("[warn] "+f, a...) }
func (stdLogger) Errorf(f string, a ...any) { stdlog.Printf("[err]  "+f, a...) }
func (stdLogger) Debugf(f string, a ...any) {}
