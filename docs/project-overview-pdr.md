# Chromefleet — Project Development Requirements

**Status:** Unreleased — Core + dual-path routing; sparse test coverage.  
**Version:** Unreleased (latest git tag v0.2.3; Phase 06 testing in progress)

## 1. Purpose & Problem Statement

**What:** Orchestrator layer on top of [chromekit](https://github.com/tuwibu/chromekit) (v0.6.1) that drives N Chrome instances with dual-path routing: native input (mouse + keyboard) under a serialized critical section when handle.Native=true, or parallel human-like CDP actions (bezier cursor, TypeHuman, ClearInput) when handle.Native=false.

**Problem solved:**
- **Dual-path routing:** Native input (Windows only) for guaranteed atomicity; CDP human-input (cross-platform) for throughput. Per-handle Native flag lets users choose per browser.
- **Input race conditions (native path):** Typing without a focused target leaks keystrokes into other windows. Clicking while the cursor moves elsewhere causes drift. Single-threaded native input worker (atomic critical section) eliminates cross-window input races.
- **Anti-bot detection (CDP path):** Parallel job submission without native guarantees, but with human-like behavior (bezier cursor glide, TypeHuman 80–220ms/char, ClearInput) to avoid detection.
- **Multi-browser scheduling:** Which job runs next? How to handle pause/resume during in-flight work? Priority queue + conditional pause/resume + hotkey listener (Pause/Resume enabled by default; Stop opt-in) provide deterministic, user-controlled sequencing.

**Why separate from chromekit?**
- chromekit is per-browser (connect, focus, evaluate, screenshot, native input).
- Multi-browser scheduling, priority queue, focus arbitration, hotkey abort are a separate domain.
- Keeping them split lets chromekit stay small and chromefleet evolve independently.

## 2. Target Users

- **Automation engineers** building Chrome-based test suites, RPA, web scraping, form submission pipelines.
- **QA teams** running headless + headed automation across multiple Chrome instances.
- **Developers** needing low-level control over native input (typing, clicking, mouse drift detection) without reinventing thread safety.

## 3. Feature Scope

### Core MVP (complete)
- **Fleet type:** Orchestrator; manages N browsers with per-handle routing, one priority queue, one native worker (when needed).
- **BrowserHandle.Native flag:** Per-browser routing decision (true → native critical section; false → parallel CDP pool).
- **Job model:** Input (BrowserID, Action, Priority, Timeout) → Output (ID, Status, Error, Duration).
- **Action types:** 
  - ClickAction, TypeAction (with optional ClearFirst bool to wipe existing value), NavigateAction.
  - On Native=true: native critical section (serial).
  - On Native=false: parallel CDP pool with human-like Mouse/Keyboard (bezier glide, TypeHuman, ClearInput).
- **Critical section (native only):** Browser.Focus → ScrollIntoView → BoundingBox → MouseMove → drift-guard → Click [±Type].
- **Priority queue:** Higher priority jobs run first; ties break FIFO.
- **Hotkey controls:** Pause (Ctrl+F10) + Resume (Ctrl+F11) enabled by default. Stop (Ctrl+Alt+Shift+S) disabled by default (opt-in).
- **Cursor drift detection (native only):** If OS cursor moves during native ops, job retried (default 3 retries, 250ms delay between attempts).
- **IME guard (native only):** Forces English keyboard layout during typing, restores on completion.
- **Worker pool:** 1 native worker (critical section, when any handle.Native=true) + N CDP workers (parallel, for Native=false + all non-Click/Type/Navigate ops).

### Out of scope
- **Deployment / CI:** Pure library; no Dockerfile, no CI/CD workflows.
- **GUI:** No UI; API only.
- **Per-browser input buffering:** All input is fleet-wide atomic.

## 4. Success Metrics

1. **No input races:** Parallel Submit() calls never leak keystrokes across windows.
2. **Cursor drift tolerance:** ±5 px (configurable via `WithDriftThresholdPx`); drift beyond threshold triggers retry.
3. **Hotkey responsiveness:** Ctrl+Alt+Shift+S cancels in-flight + pending jobs within 100 ms (blocking on next queue checkout).
4. **Pause/resume semantics:** Pause waits for current critical section to complete, then blocks worker on cond. Resume unblocks immediately.
5. **Job prioritization:** Submit 100 jobs with mixed priorities; confirm execution order matches priority desc, then insertion order.

## 5. Architecture Snapshot

```
Per-handle Native=true (native critical section):
┌────────────────┐    Submit(Job)    ┌──────────────────────┐
│  Your code     │──────────────────▶│  Fleet               │
└────────────────┘                   │   ├─ priorityQueue    │
                                     │   ├─ nativeWorker ◀─┐ (serial)
                                     │   ├─ cdpWorkers   │  (parallel)
                                     │   └─ hotkeyListener
                                     └──────────────────────┘
                                              ▼
                                     Browser.Focus
                                     ScrollIntoView
                                     BoundingBox
                                     MouseMove
                                     Drift-guard (retry default 3×)
                                     Click [±Type]

Per-handle Native=false (parallel CDP with human input):
┌────────────────┐    Submit(Job)    ┌──────────────────────┐
│  Your code     │──────────────────▶│  Fleet               │
└────────────────┘                   │   ├─ priorityQueue    │
                                     │   ├─ nativeWorker     (idle)
                                     │   ├─ cdpWorkers ◀──┐ (parallel)
                                     │   └─ hotkeyListener
                                     └──────────────────────┘
                                              ▼
                                     Mouse.Click (bezier glide)
                                     Mouse.FocusElement
                                     Keyboard.ClearInput (optional)
                                     Keyboard.TypeHuman
```

**Atomic critical section (native only, single thread):**
1. Browser.Focus (SetWindowForeground)
2. Page.ScrollIntoView
3. Page.BoundingBox
4. MouseMove (to element coords)
5. Drift-guard checkpoint (verify cursor hasn't moved; retry loop default 3 retries)
6. Click (native input)
7. [±Type with IME guard]

Splitting this sequence creates races: typing without a focused window, clicking while cursor drifts, etc.

**Parallel human-input ops (CDP workers):**
- Mouse.Click with bezier glide (anti-bot safe, smooth cursor movement).
- Mouse.FocusElement, Keyboard.ClearInput, Keyboard.TypeHuman (80–220ms/char, 5% typo rate).
- Navigate, Evaluate, Screenshot, WaitForNavigation — run in parallel across browsers.

## 6. Configuration

### Options
- `WithLogger(Logger)` — wire zap, zerolog, slog, or NoopLogger.
- `WithDefaultTimeout(time.Duration)` — per-job default (zero uses fleet default).
- `WithCDPWorkers(int)` — parallel non-native worker count (default 4).
- `WithStopHotkey(Hotkey)` — enable stop combo; default Ctrl+Alt+Shift+S (disabled by default).
- `WithStopHotkeyDisabled()` — disable stop hotkey.
- `WithPauseHotkey(Hotkey)` — override pause combo (default Ctrl+F10; enabled by default).
- `WithPauseHotkeyDisabled()` — disable pause hotkey.
- `WithResumeHotkey(Hotkey)` — override resume combo (default Ctrl+F11; enabled by default).
- `WithResumeHotkeyDisabled()` — disable resume hotkey.
- `OnStop(func(reason string))` — callback on stop.
- `OnPause(func(reason string))` — callback on pause.
- `OnResume(func(reason string))` — callback on resume.
- `WithDriftThresholdPx(int)` — cursor drift tolerance (default 5 px, native only).
- `WithDriftRetries(int)` — retry count on cursor drift (default 3, native only). Total attempts = 1 + driftRetries.
- `WithDriftRetryDelay(time.Duration)` — sleep between drift retries (default 250ms, native only).

### Platform support
- **Windows:** Full native support (focus, scroll, click, type, drift guard, IME).
- **macOS / Linux:** Fallback to CDP input (detectable by anti-bot scripts); no native input, no hotkey listener.

## 7. Dependencies

**Direct:**
- github.com/tuwibu/chromekit v0.6.1 (human input implementations: Mouse.Click bezier, TypeHuman, ClearInput)

**Indirect (transitive):**
- chromedp (cdproto v0.0.0-20260427, chromedp v0.15.1, sysutil v1.1.0)
- gobwas/ws (WebSocket)
- golang.org/x/sys v0.42.0 (Windows syscalls)

**Go version:** 1.26.2+

## 8. Known Gaps

1. **Test coverage:** <10%. Critical sections (dispatcher worker loop, pause/resume cond, hotkey listener teardown, cursor drift retry) untested.
2. **Integration tests:** No end-to-end tests combining all features (Submit → hotkey pause → hotkey resume → hotkey abort).
3. **Platform-specific tests:** Windows native input tests rely on manual verification or CI with real Chrome.
4. **Error handling:** Missing graceful degradation for edge cases (e.g., Browser.Focus fails, ScrollIntoView times out).

## 9. Next Phases

### Phase 06: Test coverage expansion
- Unit tests for critical sections (queue ordering, pause/resume cond, hotkey listener).
- Integration tests (2–3 browser stress, concurrent Submit with hotkey events).
- Mock/stub Windows API calls for cross-platform test isolation.
- **Target:** >80% coverage.

### Phase 07: Stability hardening
- Timeout handling edge cases.
- Retry logic refinement (drift, transient focus loss).
- Logging improvements for debugging.

### Phase 08: Documentation + examples
- API reference (Godoc + guides).
- Architecture diagrams (Mermaid).
- Example programs with explanations.

## 10. Deliverables

- ✅ fleet.go, dispatcher.go, action.go, job.go, hotkey.go
- ✅ Platform-specific files (_windows.go, _other.go, internal/winapi/)
- ✅ Example programs (five_browser_steps, testpage; consolidated from 9)
- ⏳ Test suite (sparse; Phase 06 in progress — <10% coverage)
- ✅ Documentation suite (README, PDR, codebase summary, system architecture, code standards, roadmap, changelog)
