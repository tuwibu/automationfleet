# Code Standards

**Language:** Go 1.26.2  
**Naming convention:** snake_case for files; PascalCase for types/funcs (Go standard)  
**Platform split:** `_windows.go` / `_other.go` with `//go:build` directives

## Naming Conventions

### Files
- **snake_case:** `dispatcher_queue.go`, `dispatcher_critical_windows.go`, `browser_handle.go`.
- **Pattern:** `<domain>_<feature>[_<platform>].go`.
  - `fleet.go` — main type.
  - `dispatcher.go`, `dispatcher_queue.go`, `dispatcher_critical_windows.go` — dispatcher domain split by feature + platform.
  - `action.go`, `job.go`, `hotkey.go` — public API types.
  - `browser_handle.go` — browser binding.

### Packages
- **Root:** `chromefleet` (orchestrator).
- **Internal:** `internal/winapi` (platform-specific Windows API wrappers).

### Types & Functions
- **PascalCase:** `Fleet`, `Dispatcher`, `Job`, `JobStatus`, `Action`, `ClickAction`, `TypeAction`, `BrowserHandle`, `Hotkey`, `ParseHotkey`.
- **Constants:** `ModCtrl`, `ModAlt`, `ModShift`, `ModWin`, `KeyA`–`KeyZ`, `KeyF1`–`KeyF12`, `StatusDone`, `StatusFailed`, `StatusCancelled`, `StatusRejected`.
- **Interfaces:** `Logger`, `Action`.
- **Unexported helpers:** `nativeWorker()`, `cdpWorker()`, `enqueue()`, `priorityQueue` (heap impl).

### Variables & Parameters
- **Lowercase + initials:** `d` (dispatcher), `f` (fleet), `j` (job), `h` (handle/hotkey), `resCh` (result channel), `mu` (mutex), `wg` (wait group).

## Platform Abstraction Pattern

### Build Tags
```go
//go:build windows
// +build windows
// (Windows-specific implementation)

//go:build !windows
// +build !windows
// (Non-Windows fallback or no-op)
```

**Files using this pattern:**
- `dispatcher_critical_windows.go` — Full native worker (focus, scroll, click, type, drift guard, IME).
- `dispatcher_critical_other.go` — CDP fallback (warn once, then delegate).
- `hotkey_windows.go` — RegisterHotkey via user32.dll.
- `hotkey_other.go` — Stub (returns immediately).
- `internal/winapi/*_windows.go` — GetCursorPos, keyboard layout.
- `internal/winapi/stubs_other.go` — Cross-platform no-ops.

### Strategy
**Compile-time split:** Go's `//go:build` directives allow the same package to export different implementations per platform without runtime branching. One binary includes only its target platform code.

**No GOOS checks in runtime:** Avoid `if runtime.GOOS == "windows"` in production code. Use file-level build tags instead.

## Error Handling

### Sentinel Errors (package-level)
```go
var (
    ErrFleetStopped   = errors.New("fleet has stopped; no new jobs accepted")
    ErrUnknownBrowser = errors.New("browser ID not registered with fleet")
    errCursorDrift    = errors.New("chromefleet: cursor drift detected") // internal, retried once
)
```

### Validation Errors
Action types validate() on construction:
```go
func (a ClickAction) validate() error {
    if a.Selector == "" {
        return errors.New("ClickAction: Selector required")
    }
    return nil
}
```

### No Panics
- Dispatcher never panics; returns error in JobResult.
- Submit validates early; returns StatusRejected if invalid.
- Unexpected conditions (e.g., nil Browser) log a warn and skip, rather than crash.

## Package Layout

### Root Package (`chromefleet`)
**Exported:**
- Types: `Fleet`, `Job`, `JobResult`, `JobID`, `JobStatus`, `Action`, `ClickAction`, `TypeAction`, `NavigateAction`, `BrowserHandle`, `Hotkey`, `Modifier`, `Key`, `Logger`, `NoopLogger`, `HotkeyBinding`.
- Functions: `New`, `ParseHotkey`, `NewHotkeyBinding`.
- Options: `WithLogger`, `WithDefaultTimeout`, `WithCDPWorkers`, `WithStopHotkey`, `WithStopHotkeyDisabled`, `WithPauseHotkey`, `WithPauseHotkeyDisabled`, `WithResumeHotkey`, `WithResumeHotkeyDisabled`, `OnStop`, `OnPause`, `OnResume`, `WithDriftThresholdPx`.
- Constants: `StatusDone`, `StatusFailed`, `StatusCancelled`, `StatusRejected`, `ModCtrl`, `ModAlt`, `ModShift`, `ModWin`, `KeyA`–`KeyZ`, `KeyF1`–`KeyF12`, `DefaultStopHotkey`, `DefaultPauseHotkey`, `DefaultResumeHotkey`.

**Unexported:**
- Types: `config`, `Dispatcher`, `queuedJob`, `priorityQueue`.
- Functions: `nativeWorker`, `cdpWorker`, `enqueue`, `deliver`, `nextJobID`, `newDispatcher`, `start`, `stop`.

**Invariant:** Fleet methods (Register, Submit, Start, Stop, Pause, Resume) hold mu locks to synchronize access to handles, queue, and dispatcher state. No public exposure of internal mutex.

### Internal Package (`internal/winapi`)
**Purpose:** Platform-specific Windows API wrappers + cross-platform stubs.

**Exported:**
- Functions (both platforms): `GetCursorPos() (x, y int, err error)`, `GetCurrentKeyboardLayout() (uint64, error)`, `ForceENUSLayout() (previousLayout uint64, err error)`, `RestoreLayout(layout uint64) error`.

**Windows-specific implementation:** Direct syscall via windows package.
**Non-Windows stubs:** Return `errors.New("unsupported on this platform")` or silent no-op (depending on function semantics).

## Code Organization Rules

### File Size Limit
- Target: ≤200 LOC per file (except markdown, config, env).
- Current state: `fleet.go` (363), `dispatcher.go` (272) exceed limit; acceptable because they are critical monoliths with high cohesion.
  - `fleet.go`: types, Options, lifecycle methods — cannot split without sacrificing clarity.
  - `dispatcher.go`: worker loop, queue checkout, pause/resume cond — single concern.
- Justification: These files have single entry point + tight internal coupling; splitting would create fragmentation.

### Module Boundary
- Public API: root package only.
- Internal only: `internal/winapi`, dispatcher internals, queue heap.
- No cross-package dependencies except chromekit.

## Test Conventions

### File Naming
- `<module>_test.go` in the same package (e.g., `hotkey_test.go` tests `hotkey.go` + `hotkey_windows.go` + `hotkey_other.go`).

### Test Function Signature
```go
func TestParseHotkey(t *testing.T) {
    h, err := ParseHotkey("Ctrl+Alt+Shift+S")
    if err != nil {
        t.Fatalf("ParseHotkey failed: %v", err)
    }
    if h.Mods != (ModCtrl | ModAlt | ModShift) || h.Key != KeyS {
        t.Errorf("expected Ctrl+Alt+Shift+S, got %v", h)
    }
}
```

### Table-Driven Tests (preferred for multiple cases)
```go
func TestParseHotkey_Cases(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Hotkey
        wantErr bool
    }{
        {"Ctrl+S", "Ctrl+S", Hotkey{ModCtrl, KeyS}, false},
        {"Alt+F4", "Alt+F4", Hotkey{ModAlt, KeyF4}, false},
        {"invalid", "BadKey", Hotkey{}, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseHotkey(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("wantErr=%v, got %v", tt.wantErr, err)
            }
            if got != tt.want {
                t.Errorf("ParseHotkey(%q) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}
```

### Platform-Specific Tests
- Use `//go:build windows` in `<module>_windows_test.go` for Windows-only tests.
- For cross-platform behavior, test in the main `_test.go` file with mocks/stubs.

## Interface Design

### Public Interfaces
- **Action** — type-switch dispatch. Implementations must validate() and provide kind() for logging.
- **Logger** — pluggable logging (Infof, Warnf, Errorf). NoopLogger for testing/silent mode.

**Rationale:** Keep interfaces small. Action dispatch via type-switch is faster than reflection and clearer than polymorphic method sets.

## Concurrency Model

### Mutexes
- **Fleet.mu (RWMutex):** Guards handles map, stopped flag. Read-lock for queries, write-lock for Register/Stop.
- **Dispatcher.mu (Mutex):** Guards queue, paused flag, insertSeq. Condition variable attached for pause/resume/queue-wake.

### Channels
- **resCh (JobResult):** Buffered channel (cap=1) returned by Submit. Dispatcher sends exactly once.
- **cdpJobs (queuedJob):** Unbuffered work queue to CDP workers.
- **hotkeyDone (struct{}):** Signal from hotkey listener when teardown completes.

### No Goroutine Leaks
- Dispatcher workers (nativeWorker, cdpWorkers, hotkey listener) block on context.Done when Fleet.Stop() is called.
- WaitGroup.Wait() in Stop() ensures all workers exit before returning.

## Logging Standards

### Dispatcher Logs (once per job)
```go
d.fleet.log.Infof("job %d: Browser=%s Action=%s", qj.id, qj.job.BrowserID, qj.job.Action.kind())
d.fleet.log.Infof("job %d: done in %v", qj.id, time.Since(start))
d.fleet.log.Errorf("job %d: failed: %v", qj.id, err)
d.fleet.log.Warnf("job %d: cursor drift detected; retrying", qj.id)
```

### Hotkey Listener Logs
```go
d.fleet.log.Infof("stop hotkey %v fired; aborting all jobs", d.fleet.cfg.stopHotkey)
d.fleet.log.Warnf("hotkey listener teardown: %v", err)
```

### Design Rationale
- Log once per significant event (job completion, error, hotkey fire).
- Avoid spam (e.g., no log per queue peek; only on dequeue or fire).
- Use job ID for traceability across log lines.

## Config Builder Pattern (Option)

**Pattern:**
```go
type Option func(*config)

func WithLogger(l Logger) Option {
    return func(c *config) {
        if l != nil {
            c.logger = l
        }
    }
}

f := New(WithLogger(myLogger), WithDefaultTimeout(5*time.Second))
```

**Rationale:** Extensible; new options don't break existing code. Nil checks prevent accidental resets.

## Type Validation

### Explicit validate() on Unsafe Types
- `BrowserHandle.validate()`: ID required, Browser required, Scale > 0.
- `Action.validate()`: subtype-specific rules (Selector required, Text required).

**Called by:** Fleet.Register(), Dispatcher.enqueue().

### At-Construction Validation
```go
h := &BrowserHandle{...}
if err := h.validate(); err != nil {
    return err // explicit error bubbling
}
```

## Performance Considerations

### Memory
- Jobs are immutable input structs; safe to share via queue.
- Result channels are buffered (cap=1); no goroutine stalling on send.
- Queue uses heap.Interface (sort.Heap); O(log N) insert/pop.

### CPU
- Dispatcher critical section is *single-threaded* (one nativeWorker).
- CDP workers are *parallel* (N goroutines).
- No busy-loops; all waiting is on cond, channels, or context.Done.

### Latency
- Pause/Resume uses conditional; O(1) wake/block.
- Queue-to-dispatch is O(log N) heap pop.
- Hotkey abort is synchronous; in-flight job finishes cleanly, pending jobs drop immediately.

## Dependencies

### Import Rules
- Root package imports only chromekit (for Browser, Page types).
- Internal/winapi imports windows/syscall (platform-specific).
- No external logging, metrics, or config libraries; Logger interface is user-provided.

### Rationale
- Keep deps minimal; library users provide their own logger.
- No transitive drag on consumers.

## Build & Module Management

### go.mod
- `go 1.26.2`.
- `require github.com/tuwibu/chromekit v0.2.0`.
- `replace github.com/tuwibu/chromekit => ../chromekit` (local dev only; remove before publishing).

### Build Tags for Platform Safety
- Single binary: includes only target platform code.
- No runtime platform checks; all branching is compile-time.

## Documentation Standards

### Exported Types/Functions
- **Godoc comment on public items.** Example:
  ```go
  // Fleet is the public orchestrator. Register browsers, Submit jobs, and Stop when done.
  type Fleet struct { ... }

  // New creates a Fleet with optional config.
  func New(opts ...Option) *Fleet { ... }
  ```

- **Unexported types:** Brief comments explaining role (e.g., `// Dispatcher owns the queue and worker pool.`).

### Examples
- **In examples/ subdirectory:** Full working programs with comments explaining intent.
- **In Godoc:** Code snippets in comments should be compilable (not required; helpful for clarity).

## Code Review Checklist

- [ ] No panics; errors returned explicitly.
- [ ] Mutex locks held only as long as needed.
- [ ] No goroutine leaks (WaitGroup.Wait called in Stop).
- [ ] Channels are either buffered with a known cap or blocked on context.
- [ ] Action types validate() before dispatch.
- [ ] Fleet.mu/Dispatcher.mu held for critical sections.
- [ ] Platform-specific code guarded by //go:build directives.
- [ ] Tests table-driven where possible; cover edge cases.
- [ ] Log messages include job ID for traceability.
