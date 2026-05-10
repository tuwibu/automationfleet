# Codebase Summary

**Root package:** `chromefleet`  
**Total root .go files:** 11 (1521 LOC)  
**Internal packages:** `internal/winapi` (3 files)  
**Examples:** 9 programs  
**Go version:** 1.26.2

## Root Package Structure

### Core Types & Lifecycle

| File | LOC | Purpose |
|------|-----|---------|
| **fleet.go** | 363 | Fleet orchestrator; New, Register, Start, Stop, Pause, Resume, Submit, Wait. Config builder (Option pattern with defaults). |
| **dispatcher.go** | 272 | Worker loop: priority queue checkout, native/CDP routing, critical section orchestration, pause/resume cond. |
| **action.go** | 112 | Action interface (kind, validate); ClickAction, TypeAction, NavigateAction enum-like dispatch. |
| **hotkey.go** | 150 | Hotkey struct (Mods, Key), ParseHotkey string parsing, listener lifecycle. Modifier + Key constants (KeyA–Z, KeyF1–F12). |
| **job.go** | 75 | JobID, JobStatus enum (Done/Failed/Cancelled/Rejected), Job input, JobResult output. |
| **browser_handle.go** | 34 | BrowserHandle struct (ID, Browser ptr, X, Y, Scale) + validation. Fleet-stable browser binding. |

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
- Work: `Register(handle *BrowserHandle)`, `Submit(job Job) chan JobResult`, `AbortAll()`.
- Control: `Pause()`, `Resume()`.

**Job** — Work unit  
- Input: `BrowserID string`, `Action`, `Priority int`, `Timeout time.Duration`.

**JobResult** — Execution outcome  
- `ID JobID`, `BrowserID string`, `Status JobStatus`, `Err error`, `Took time.Duration`.

**JobStatus** — Terminal enum  
- `StatusDone`, `StatusFailed`, `StatusCancelled`, `StatusRejected`.

**Action** — Interface (kind, validate)  
- `ClickAction{Selector, Button}`.
- `TypeAction{Selector, Text}` — requires Selector (no cross-window typing).
- `NavigateAction{URL}` — omnibox-driven.

**BrowserHandle** — Fleet-stable browser binding  
- `ID string`, `Browser *chromekit.Browser`, `X, Y int`, `Scale float64`.

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

### Errors

- `ErrFleetStopped` — Submit after Stop/AbortAll.
- `ErrUnknownBrowser` — Job.BrowserID not registered.

## Examples Directory

| Subdirectory | Purpose | Key features |
|---|---|---|
| **hotkey_demo** | Submit N jobs; user presses Ctrl+Alt+Shift+S to abort. Verifies in-flight finishes critical section, pending get StatusCancelled. | Hotkey abort, job lifecycle tracking. |
| **stress_nine** | Launch ≤9 Chrome instances (3×3 grid); N random jobs per browser. Print p50/p95/p99 latency + error counts. | Parallel job submission, latency profiling, stress tolerance. |
| **two_browser** | 2 Chrome side-by-side; 20 alternating click+type jobs. Verify no cross-window input leak. | Event log isolation, input safety. |
| **nine_navigate** | Navigate 9 browsers concurrently via omnibox (e.g., google.com, github.com). | NavigateAction, multi-browser scheduling. |
| **pause_resume_demo** | Hotkey pause (Ctrl+F10) / resume (Ctrl+F11) flow demonstration. | Pause/resume semantics, conditional blocking. |
| **stress_omnibox_click** | Repeated omnibox navigation + clicks under load. | Stress tolerance, navigation stability. |
| **pid_smoke** | Smoke test involving PID handling (purpose from code inspection). | PID lifecycle, resource cleanup. |
| **omnibox_smoke** | Light omnibox navigation smoke test. | Quick sanity check. |
| **testpage** | Shared test server (HTML form page) for input tests. | Test fixture, local dev server. |

## Entry Points

**Library, not CLI.**
- No `func main()` in root package.
- Public entry: `Fleet.New() → Register() → Start() → Submit() → Stop()`.
- All examples have independent `main()` for different test scenarios.

## Dependencies

**Direct:**
- `github.com/tuwibu/chromekit` v0.2.0 (local `replace ../chromekit`)

**Indirect (transitive from chromekit):**
- chromedp/cdproto (Chrome DevTools Protocol)
- chromedp/chromedp (Go CDP client)
- chromedp/sysutil (platform utilities)
- gobwas/ws (WebSocket)
- golang.org/x/sys (Windows syscall bindings)
- go-json-experiment/json

## Critical Paths

### 1. Submit → Execute (Happy Path)
```
Fleet.Submit(Job)
  → Dispatcher.enqueue(Job)
    → Validate Job.BrowserID, Action
    → Insert into priorityQueue heap
    → Wake native worker (cond.Signal)
  ← resCh = make(chan JobResult, 1)

Dispatcher.nativeWorker() loop
  ← Next job from queue (highest priority, FIFO on tie)
  → Determine action type (click, type, navigate)
  → Type-switch to handler
    - ClickAction: Browser.Focus → ScrollIntoView → BoundingBox → MouseMove → drift-guard → Click
    - TypeAction: same + IME-guard → Type
    - NavigateAction: CDP omnibox input (parallel, no critical section)
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

### 3. Hotkey Abort (Destructive)
```
User presses Ctrl+Alt+Shift+S (stop hotkey, enabled via WithStopHotkey)
  → Dispatcher.AbortAll()
    → In-flight job: run to critical-section boundary, finish cleanly
    → Pending jobs in queue: drop, deliver StatusCancelled
    → No resume after stop (destructive)
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
│   ├── hotkey_demo/
│   ├── stress_nine/
│   ├── two_browser/
│   ├── nine_navigate/
│   ├── pause_resume_demo/
│   ├── stress_omnibox_click/
│   ├── pid_smoke/
│   ├── omnibox_smoke/
│   └── testpage/
├── go.mod
├── go.sum
└── README.md
```

## How to Use (Quick Reference)

1. **Import:** `import "github.com/tuwibu/chromefleet"`
2. **Create Fleet:** `f := chromefleet.New(opts...)`
3. **Register browsers:** `f.Register(&BrowserHandle{ID: "b1", Browser: b1, X: 0, Y: 0, Scale: 1.0})`
4. **Start:** `f.Start()`
5. **Submit jobs:** `resCh := f.Submit(Job{BrowserID: "b1", Action: ClickAction{...}, Priority: 5})`
6. **Wait:** `result := <-resCh` (StatusDone, StatusFailed, StatusCancelled, StatusRejected)
7. **Stop:** `f.Stop()`

See examples/ for full runnable patterns.
