# Codebase Summary

**Root package:** `chromefleet`  
**Total root .go files:** 12 (1656 LOC)  
**Internal packages:** `internal/winapi` (3 files)  
**Examples:** 2 programs (consolidated from 9)  
**Go version:** 1.26.2

## Root Package Structure

### Core Types & Lifecycle

| File | LOC | Purpose |
|------|-----|---------|
| **fleet.go** | 391 | Fleet orchestrator; New, Register, Start, Stop, Pause, Resume, Submit, Wait. Config builder (Option pattern with defaults). WithDriftRetries / WithDriftRetryDelay for retry tuning. |
| **dispatcher.go** | 314 | Worker loop: priority queue checkout, per-handle Native routing (native worker vs CDP pool), critical section orchestration, pause/resume cond. Human input on CDP path (Mouse.Click bezier, Mouse.FocusElement, Keyboard.ClearInput, Keyboard.TypeHuman). Drift retry loop (executeCriticalWithRetry). |
| **action.go** | 117 | Action interface (kind, validate); ClickAction, TypeAction (with ClearFirst bool), NavigateAction. needsNativeCriticalSection detector for routing. |
| **hotkey.go** | 150 | Hotkey struct (Mods, Key), ParseHotkey string parsing, listener lifecycle. Modifier + Key constants (KeyA–Z, KeyF1–F12). |
| **job.go** | 75 | JobID, JobStatus enum (Done/Failed/Cancelled/Rejected), Job input, JobResult output. |
| **browser_handle.go** | 47 | BrowserHandle struct (ID, Browser ptr, X, Y, Scale, Native bool) + validation. Per-handle Native flag gates native-critical routing vs parallel CDP path. validate() enforces handle.Native matches Browser.InputBackend(). |

### Platform Abstraction

| File | LOC | Purpose |
|------|-----|---------|
| **dispatcher_critical_windows.go** | 154 | Native worker (Windows only): Focus, ScrollIntoView, BoundingBox, MouseMove, drift-guard, Click, Type, IME layout guard. |
| **dispatcher_critical_other.go** | 44 | Non-Windows fallback: single warn, then delegate to CDP input (no native guarantees). |
| **hotkey_windows.go** | 116 | RegisterHotkey via user32.dll, ListenHotkey WM_HOTKEY listener loop. |
| **hotkey_other.go** | 24 | Non-Windows stubs: ListenHotkey returns immediately, no-op. |

**Build tags:** `//go:build windows` / `//go:build !windows` partition code cleanly.

### Infrastructure

| File | LOC | Purpose |
|------|-----|---------|
| **dispatcher_queue.go** | 57 | priorityQueue heap impl (sort.Interface). Priority desc, insertion sequence FIFO tiebreak. Push, Pop, Len, Less, Swap. |

## Internal Packages

### internal/winapi

Windows API wrappers (syscall proxies) + cross-platform stubs.

| File | Purpose |
|------|---------|
| **cursor_pos_windows.go** | GetCursorPos() via user32.dll; detects human cursor interference mid-job. |
| **keyboard_layout_windows.go** | GetCurrentKeyboardLayout, ForceENUSLayout, RestoreLayout via user32.dll; IME-guard for non-Latin layouts. |
| **stubs_other.go** | Non-Windows placeholders; all funcs return "unsupported" error or silent no-op. |

## Public API Surface (Root Package)

### Types

**Fleet** — Orchestrator  
- Lifecycle: `New(opts ...Option) *Fleet`, `Start()`, `Stop()`, `Wait()`.
- Work: `Register(handle *BrowserHandle) error`, `Submit(job Job) (<-chan JobResult, error)`.
- Control: `Pause(reason string)`, `Resume(reason string)`.

**Job** — Work unit  
- Input: `BrowserID string`, `Action`, `Priority int`, `Timeout time.Duration`.

**JobResult** — Execution outcome  
- `ID JobID`, `BrowserID string`, `Status JobStatus`, `Err error`, `Took time.Duration`.

**JobStatus** — Terminal enum  
- `StatusDone`, `StatusFailed`, `StatusCancelled`, `StatusRejected`.

**Action** — Interface (kind, validate)  
- `ClickAction{Selector, Button}`.
- `TypeAction{Selector, Text, ClearFirst bool}` — requires Selector (no cross-window typing). ClearFirst=true wipes existing value via Ctrl+A→Delete before typing.
- `NavigateAction{URL, Timeout}` — native-critical when handle.Native; parallel CDP fallback when handle.Native=false.

**BrowserHandle** — Fleet-stable browser binding  
- `ID string`, `Browser *chromekit.Browser`, `X, Y int`, `Scale float64`, `Native bool`.
- `Native=true`: Browser MUST be launched with `chromekit.WithInputBackend(BackendNative)` + `WithNativeWindow(X, Y, Scale)` matching struct fields. Click/Type/Navigate route through native worker with cursor drift checks.
- `Native=false` (default): actions run on parallel CDP pool; X/Y/Scale ignored. Click/Type/Navigate execute via human Mouse/Keyboard (bezier glide, TypeHuman, ClearInput) — anti-bot safe but no drift guard.

**Hotkey** — Key combo  
- `Mods Modifier` (bitmask: ModCtrl, ModAlt, ModShift, ModWin).
- `Key Key` (VK_*: KeyA–Z, KeyF1–F12, + raw VK constants).
- Constants: `DefaultStopHotkey` (Ctrl+Alt+Shift+S), `DefaultPauseHotkey` (Ctrl+F10), `DefaultResumeHotkey` (Ctrl+F11).

**HotkeyBinding** — Listener registration  
- `Hotkey`, `OnFire func() error`.

**Logger** — Pluggable logging  
- `Infof(format string, args ...any)`, `Warnf(...)`, `Errorf(...)`.
- `NoopLogger` — silent fallback.

### Functions

- `ParseHotkey(s string) (Hotkey, error)` — parse "Ctrl+Alt+Shift+S" format.
- `NewHotkeyBinding(h Hotkey, cb func() error) *HotkeyBinding`.

### Options (Config Builder)

- `WithLogger(Logger) Option`
- `WithDefaultTimeout(time.Duration) Option`
- `WithCDPWorkers(int) Option`
- `WithStopHotkey(Hotkey) Option`, `WithStopHotkeyDisabled() Option`
- `WithPauseHotkey(Hotkey) Option`, `WithPauseHotkeyDisabled() Option`
- `WithResumeHotkey(Hotkey) Option`, `WithResumeHotkeyDisabled() Option`
- `OnStop(func(reason string)) Option`
- `OnPause(func(reason string)) Option`
- `OnResume(func(reason string)) Option`
- `WithDriftThresholdPx(int) Option`
- `WithDriftRetries(int) Option` — max retries on cursor drift (default 3). Total attempts = 1 + driftRetries.
- `WithDriftRetryDelay(time.Duration) Option` — sleep between drift retry attempts (default 250ms).

### Errors

- `ErrFleetStopped` — Submit after Stop/AbortAll.
- `ErrUnknownBrowser` — Job.BrowserID not registered.

## Examples Directory

| Subdirectory | Purpose | Key features |
|---|---|---|
| **five_browser_steps** | Launch 5 Chrome instances in parallel; regex field + TypeAction.ClearFirst test + native vs CDP routing validation. Regression test for dual-path (Native flag). | Parallel browser setup, per-handle Native flag, ClearFirst field. |
| **testpage** | Shared HTML test server (form fixture). Run before integration tests to serve localhost:8080/testpage. | Test fixture, local dev server. |

*Archived (deleted): hotkey_demo, stress_nine, two_browser, nine_navigate, pause_resume_demo, stress_omnibox_click, pid_smoke, omnibox_smoke — consolidation to core regression test suite.*

## Entry Points

**Library, not CLI.**
- No `func main()` in root package.
- Public entry: `Fleet.New() → Register() → Start() → Submit() → Stop()`.
- All examples have independent `main()` for different test scenarios.

## Dependencies

**Direct:**
- `github.com/tuwibu/chromekit` v0.6.1

**Indirect (transitive from chromekit):**
- chromedp/cdproto (Chrome DevTools Protocol)
- chromedp/chromedp (Go CDP client)
- chromedp/sysutil (platform utilities)
- gobwas/ws (WebSocket)
- golang.org/x/sys (Windows syscall bindings)
- go-json-experiment/json

## Critical Paths

### 1. Submit → Execute (Happy Path with Per-Handle Native Routing)
```
Fleet.Submit(Job)
  → Dispatcher.enqueue(Job)
    → Validate Job.BrowserID, Action
    → Insert into priorityQueue heap
    → Wake dispatcher (cond.Signal)
  ← resCh = make(chan JobResult, 1)

Dispatcher.nativeWorker() loop (executes critical-section actions)
  ← Next job from queue (highest priority, FIFO on tie)
  → Determine action type (click, type, navigate)
  → If needsNativeCriticalSection(action) && handle.Native:
    - ClickAction: Browser.Focus → ScrollIntoView → BoundingBox → MouseMove → drift-guard → executeCriticalWithRetry (retry up to 1+driftRetries on errCursorDrift)
    - TypeAction: same as Click, then Mouse.FocusElement → optional ClearInput (if ClearFirst=true) → TypeHuman (80–220ms/char)
    - NavigateAction: native omnibox input (Ctrl+L → Ctrl+A → type URL → Enter)
  → Else if handle.Native=false (default):
    - Route to cdpWorkers pool (parallel, human Mouse/Keyboard via chromekit)
    - Mouse.Click(sel) with bezier glide (anti-bot safe)
    - FocusElement(sel) → optional ClearInput → TypeHuman(text)
    - Navigate(url, timeout)
  → resCh ← JobResult{Status: Done, Took: duration}
  ← resCh closes when main sends
```

### 2. Pause/Resume Semantics
```
User presses Ctrl+F10 (pause hotkey)
  → Dispatcher.Pause()
    → d.mu.Lock()
    → d.paused = true
    → Wait for in-flight job to complete
    → Cond.Wait() blocks dispatcher on queue checkout

User presses Ctrl+F11 (resume hotkey)
  → Dispatcher.Resume()
    → d.mu.Lock()
    → d.paused = false
    → Cond.Broadcast() unblocks dispatcher
    → Next queue checkout proceeds normally
```

### 3. Stop (Hotkey or Programmatic)
```
User presses Ctrl+Alt+Shift+S (stop hotkey, if enabled via WithStopHotkey)
  OR Fleet.Stop() called programmatically
  → Fleet.requestStop (internal)
    → In-flight job: run to critical-section boundary, finish cleanly
    → Pending jobs in queue: drop, deliver StatusCancelled
    → Dispatcher loop exits, workers clean up
    → No resume after stop (destructive)

Note: Stop hotkey is DISABLED by default. Pause (Ctrl+F10) and Resume (Ctrl+F11) are enabled by default.
```

## Test Coverage

| File | Tests | Coverage |
|------|-------|----------|
| **dispatcher_queue_test.go** | 5 cases (queue order, drain, hotkey parse) | Heap impl only |
| **hotkey_test.go** | 4 cases (parse F10+Ctrl, render) | Hotkey parse/render only |
| **Total** | 9 tests | <10% of codebase |

**Gaps:** No tests for dispatcher worker loop, action execution, pause/resume cond, cursor drift, IME guard, or hotkey listener lifecycle.

## Platform-Specific Build Matrix

| Target | Native Input | Hotkey Listener | Notes |
|--------|---|---|---|
| Windows | ✅ Full | ✅ Full (user32.dll) | Production-grade |
| macOS / Linux | ⚠️ CDP fallback | ❌ No-op | Detectable by anti-bot; no native input guarantees |

## File Organization

```
chromefleet/
├── fleet.go                              # Orchestrator + Options
├── dispatcher.go                         # Worker loop + queue checkout
├── dispatcher_queue.go                   # Priority heap impl
├── dispatcher_critical_windows.go        # Native input (Windows)
├── dispatcher_critical_other.go          # CDP fallback (non-Windows)
├── action.go                             # Action interface + impls
├── job.go                                # Job + JobStatus enums
├── hotkey.go                             # Hotkey + ParseHotkey
├── hotkey_windows.go                     # RegisterHotkey impl
├── hotkey_other.go                       # Hotkey stubs
├── browser_handle.go                     # BrowserHandle struct
├── internal/
│   └── winapi/
│       ├── cursor_pos_windows.go         # GetCursorPos
│       ├── keyboard_layout_windows.go    # IME guard
│       └── stubs_other.go                # Cross-platform stubs
├── examples/
│   ├── five_browser_steps/
│   └── testpage/
├── go.mod
├── go.sum
└── README.md
```

## How to Use (Quick Reference)

1. **Import:** `import "github.com/tuwibu/chromefleet"`
2. **Create Fleet:** `f := chromefleet.New(opts...)`
3. **Register browsers:** `f.Register(&BrowserHandle{ID: "b1", Browser: b1, X: 0, Y: 0, Scale: 1.0, Native: true})` (set Native=true if browser is launched with BackendNative; false otherwise)
4. **Start:** `f.Start()`
5. **Submit jobs:** `resCh := f.Submit(Job{BrowserID: "b1", Action: ClickAction{...}, Priority: 5})`
6. **Wait:** `result := <-resCh` (StatusDone, StatusFailed, StatusCancelled, StatusRejected)
7. **Stop:** `f.Stop()`

See examples/ for full runnable patterns.
