# chromefleet

Orchestrator on top of [chromekit](../chromekit) for driving N Chrome instances with native input (mouse + keyboard) under a serialized queue.

**Why a separate repo?** chromekit is a per-browser library. Multi-browser scheduling — priority queue, focus arbitration, hotkey abort — is a different concern. Keeping them split lets chromekit stay small and chromefleet evolve independently.

## Architecture

```
┌────────────────┐    Submit(Job)    ┌──────────────────────┐
│  Your code     │──────────────────▶│  Fleet               │
└────────────────┘                   │   ├─ priorityQueue    │
                                     │   ├─ 1× nativeWorker  │── critical section ──▶ chromekit.Browser
                                     │   ├─ N× cdpWorkers    │── parallel ─────────▶ chromekit.Page
                                     │   └─ hotkeyListener   │── Ctrl+Alt+Shift+S ─▶ AbortAll
                                     └──────────────────────┘
```

**Atomic critical section** (single native worker, fleet-wide):

```
Browser.Focus → page.ScrollIntoView → page.BoundingBox → MouseMove → drift-guard → Click [+ IME-guard → Type]
```

Cannot be split — typing without a focused target risks keystrokes leaking into other windows.

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
fleet.Register(&chromefleet.BrowserHandle{ID: "b1", Browser: b1, X: 0, Y: 0, Scale: 1.0})

ch, _ := fleet.Submit(chromefleet.Job{
    BrowserID: "b1",
    Action:    chromefleet.TypeAction{Selector: "input[name=q]", Text: "hello"},
    Priority:  5,
})
result := <-ch
```

## Hotkey abort

Default `Ctrl+Alt+Shift+S` cancels every in-flight + pending job. Customize:

```go
hk, _ := chromefleet.ParseHotkey("Ctrl+Shift+Q")
fleet := chromefleet.New(chromefleet.WithStopHotkey(hk))
```

Disable entirely: `chromefleet.WithStopHotkeyDisabled()`.

## Development

This repo uses a `replace` directive pointing to `../chromekit`:

```
replace github.com/tuwibu/chromekit => ../chromekit
```

Remove before publishing.

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
