# chromefleet

Orchestrator on top of [chromekit](https://github.com/tuwibu/chromekit) (v0.6.1) for driving N Chrome instances with dual-path routing: native input (mouse + keyboard) under a serialized critical section, or parallel human-like CDP actions (bezier cursor, TypeHuman, ClearInput) per per-handle `Native` flag.

**Why a separate repo?** chromekit is a per-browser library. Multi-browser scheduling — priority queue, per-handle routing, focus arbitration, hotkey controls — is a different concern. Keeping them split lets chromekit stay small and chromefleet evolve independently.

## Architecture

```
┌────────────────┐    Submit(Job)    ┌──────────────────────────────┐
│  Your code     │──────────────────▶│  Fleet                       │
└────────────────┘                   │   ├─ priorityQueue           │
                                     │   ├─ nativeWorker (if any)   │
                                     │   ├─ cdpWorkers pool (if any)│
                                     │   └─ hotkeyListener          │
                                     │      (Pause/Resume enabled   │
                                     │       by default; Stop opt-in)
                                     └──────────────────────────────┘
                    ▲                 ▼
              Per-handle Native flag determines routing:
              
  Native=true (native critical section):      Native=false (parallel CDP):
  ├─ 1× nativeWorker (serial)                 ├─ N× cdpWorkers (parallel)
  ├─ Focus, scroll, bbox, drift-guard        ├─ Mouse.Click (bezier glide)
  ├─ Click / Type / Navigate                  ├─ Mouse.FocusElement
  ├─ Retry on drift (default 3)               ├─ Keyboard.ClearInput (optional)
  └─ IME guard (Windows)                      └─ Keyboard.TypeHuman
                                                 (80–220ms/char, anti-bot safe)
```

**Native critical section** (single worker, one NativeHandle at a time):

```
Browser.Focus → page.ScrollIntoView → page.BoundingBox → MouseMove → drift-guard → Click [+ IME-guard → Type]
```

Cannot be split — typing without a focused target risks keystrokes leaking into other windows.

**CDP parallel path** (multiple browsers concurrently):

```
Mouse.Click (bezier glide) + FocusElement + optional ClearInput + TypeHuman (human-like 80–220ms/char, 5% typo)
```

Each browser runs independently on a CDP worker; human-like behavior avoids bot detection.

## Quick start

```go
fleet := chromefleet.New(
    chromefleet.WithLogger(myLogger),
    chromefleet.WithDefaultTimeout(10*time.Second),
)
fleet.Start()
defer fleet.Stop()

b1, _ := chromekit.Connect(9222,
    chromekit.WithInputBackend(chromekit.BackendNative),
    chromekit.WithNativeWindow(0, 0, 1.0))
fleet.Register(&chromefleet.BrowserHandle{
    ID: "b1", Browser: b1, X: 0, Y: 0, Scale: 1.0, Native: true,
})

ch, _ := fleet.Submit(chromefleet.Job{
    BrowserID: "b1",
    Action:    chromefleet.TypeAction{Selector: "input[name=q]", Text: "hello"},
    Priority:  5,
})
result := <-ch
```

## Hotkey controls

**Pause (Ctrl+F10) and Resume (Ctrl+F11):** Enabled by default. Pause gracefully blocks after the current job completes; Resume unblocks immediately.

**Stop (Ctrl+Alt+Shift+S):** DISABLED by default (opt-in via `WithStopHotkey`). Destructive — no resume after stop.

Customize stop hotkey:

```go
hk, _ := chromefleet.ParseHotkey("Ctrl+Shift+Q")
fleet := chromefleet.New(chromefleet.WithStopHotkey(hk))
```

Or programmatically: `fleet.Pause("reason")` / `fleet.Resume("reason")` / `fleet.Stop()`.

## Dependencies

- `github.com/tuwibu/chromekit` v0.6.1 — per-browser library for native input + CDP fallback

<!-- stale: verify whether published version or local development. go.mod shows v0.6.1 require; no replace directive seen. -->

## Status

| Phase | Status |
|------|--------|
| 01 — chromekit prep (mutex, HWND, Focus) | ✅ done |
| 02 — chromefleet bootstrap (types, skeleton) | ✅ done |
| 03 — dispatcher (queue, critical section) | ✅ done |
| 04 — hotkey listener | ✅ done |
| 05 — PoC (2-browser, stress 9, hotkey demo) | ✅ done |
| 06 — testing & stability (>80% coverage) | ⏳ in progress |
| 07 — hardening (timeouts, retry, logging) | ⏳ planned |
| 08 — documentation + examples | ⏳ planned |

See [`docs/development-roadmap.md`](docs/development-roadmap.md) for detailed phase breakdown.

## Platform support

- **Windows**: full native input + hotkey listener.
- **Linux / macOS**: CDP fallback (no native cursor / hotkey). Fleet stays functional but loses the anti-bot benefits.
