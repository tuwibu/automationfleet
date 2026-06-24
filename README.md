# automationfleet

Orchestrator for driving N browser instances — **Chrome** (via [chromekit](https://github.com/tuwibu/chromekit)) and **Firefox** (via [firefoxkit](https://github.com/tuwibu/firefoxkit)) — through one fleet, with dual-path routing: native input (mouse + keyboard) under a serialized critical section, or parallel human-like remote actions (bezier cursor, TypeHuman, ClearInput) per per-handle `Native` flag.

**Driver abstraction.** The fleet talks to a `Driver` interface, not a concrete kit. Two thin adapters (`WrapChrome`, `WrapFirefox`) bridge chromekit/firefoxkit into the fleet; the kits stay unmodified and independently versioned. The dispatcher, queue, focus arbitration, and hotkey controls are kit-agnostic.

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
fleet := automationfleet.New(
    automationfleet.WithLogger(myLogger),
    automationfleet.WithDefaultTimeout(10*time.Second),
)
fleet.Start()
defer fleet.Stop()

b1, _ := chromekit.Connect(9222,
    chromekit.WithInputBackend(chromekit.BackendNative),
    chromekit.WithNativeWindow(0, 0, 1.0))
fleet.RegisterChrome("b1", b1, true, 0, 0, 1.0) // id, browser, native, x, y, scale

ch, _ := fleet.Submit(automationfleet.Job{
    BrowserID: "b1",
    Action:    automationfleet.TypeAction{Selector: "input[name=q]", Text: "hello"},
    Priority:  5,
})
result := <-ch
```

## Hotkey controls

**Pause (Ctrl+F10) and Resume (Ctrl+F11):** Enabled by default. Pause gracefully blocks after the current job completes; Resume unblocks immediately.

**Stop (Ctrl+Alt+Shift+S):** DISABLED by default (opt-in via `WithStopHotkey`). Destructive — no resume after stop.

Customize stop hotkey:

```go
hk, _ := automationfleet.ParseHotkey("Ctrl+Shift+Q")
fleet := automationfleet.New(automationfleet.WithStopHotkey(hk))
```

Or programmatically: `fleet.Pause("reason")` / `fleet.Resume("reason")` / `fleet.Stop()`.

## Drivers

Register either kit through a typed helper — both wrap the browser in a `Driver` and route through the same dispatcher:

```go
fleet.RegisterChrome("chrome-1", chromeBrowser, true,  0, 0, 1.0) // native input path
fleet.RegisterFirefox("firefox-1", ffBrowser,  false, 0, 0, 1.0) // Remote (BiDi) path
```

See [`examples/mixed_fleet`](examples/mixed_fleet/main.go) for a Chrome + Firefox fleet.

**Firefox native input caveat:** firefoxkit builds its native input window without the content offset, so CSS (0,0) maps to the window top-left (tabs/omnibox) rather than the content origin. Register firefox with `native=false` (BiDi/Remote) — fully supported — until firefoxkit wires `MeasureContentOffset` into its native window. Chrome native input is unaffected.

## Dependencies

- `github.com/tuwibu/chromekit` v0.6.1 — per-browser Chrome library (native input + CDP).
- `github.com/tuwibu/firefoxkit` — per-browser Firefox library (native input + BiDi). **Unpublished**: consumed via `replace => ../firefoxkit`. This repo is **monorepo-internal** — clone all three repos side-by-side; the relative replace breaks external `go get`.

> `go.mod` requires chromekit v0.6.1; the local `../chromekit` checkout may be ahead (e.g. v0.7.0). The pinned network version is what builds/tests run against.

## Status

| Phase | Status |
|------|--------|
| 01 — chromekit prep (mutex, HWND, Focus) | ✅ done |
| 02 — automationfleet bootstrap (types, skeleton) | ✅ done |
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
