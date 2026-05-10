# Chromefleet — Project Development Requirements

**Status:** Library complete; sparse test coverage.  
**Version:** v0.2.0 (pending test expansion)

## 1. Purpose & Problem Statement

**What:** Orchestrator layer on top of [chromekit](../chromekit) that drives N Chrome instances with serialized native input (mouse + keyboard) under a priority queue.

**Problem solved:**
- **Input race conditions:** Typing without a focused target leaks keystrokes into other windows. Clicking while the cursor moves elsewhere causes drift. Single-threaded native input worker (atomic critical section) eliminates cross-window input races.
- **Multi-browser scheduling:** Which job runs next? How to handle Ctrl+Alt+Shift+S abort during in-flight work? Priority queue + conditional pause/resume + hotkey listener provide deterministic, user-controlled sequencing.

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
- **Fleet type:** Orchestrator; manages N browsers, one priority queue, one native worker.
- **Job model:** Input (BrowserID, Action, Priority, Timeout) → Output (ID, Status, Error, Duration).
- **Action types:** ClickAction, TypeAction, NavigateAction (omnibox-driven).
- **Critical section:** Browser.Focus → ScrollIntoView → BoundingBox → MouseMove → drift-guard → Click [±Type].
- **Priority queue:** Higher priority jobs run first; ties break FIFO.
- **Hotkey listener:** Ctrl+Alt+Shift+S (abort), Ctrl+F10 (pause), Ctrl+F11 (resume).
- **Cursor drift detection:** If OS cursor moves during native ops, job retried once.
- **IME guard:** Forces English keyboard layout during typing, restores on completion.
- **Worker pool:** 1 native worker (critical section) + N CDP workers (parallel non-native ops).

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
┌────────────────┐    Submit(Job)    ┌──────────────────────┐
│  Your code     │──────────────────▶│  Fleet               │
└────────────────┘                   │   ├─ priorityQueue    │
                                     │   ├─ 1× nativeWorker  │── critical section ──▶ Browser.Focus
                                     │   ├─ N× cdpWorkers    │── parallel ─────────▶ Page.Evaluate
                                     │   └─ hotkeyListener   │── Ctrl+Alt+Shift+S ─▶ AbortAll
                                     └──────────────────────┘
```

**Atomic critical section (single thread, fleet-wide):**
1. Browser.Focus (SetWindowForeground)
2. Page.ScrollIntoView
3. Page.BoundingBox
4. MouseMove (to element coords)
5. Drift-guard checkpoint (verify cursor hasn't moved)
6. Click (native input)
7. [±Type with IME guard]

Splitting this sequence creates races: typing without a focused window, clicking while cursor drifts, etc.

**Non-native ops (CDP workers, parallel):**
- Navigate, Evaluate, Screenshot, WaitForNavigation, etc. — no native input risk.

## 6. Configuration

### Options
- `WithLogger(Logger)` — wire zap, zerolog, slog, or NoopLogger.
- `WithDefaultTimeout(time.Duration)` — per-job default (zero uses fleet default).
- `WithCDPWorkers(int)` — parallel non-native worker count (default 4).
- `WithStopHotkey(Hotkey)` — enable abort combo; default Ctrl+Alt+Shift+S.
- `WithStopHotkeyDisabled()` — disable abort entirely.
- `WithPauseHotkey(Hotkey)` — override pause combo (default Ctrl+F10).
- `WithPauseHotkeyDisabled()` — disable pause listener.
- `WithResumeHotkey(Hotkey)` — override resume combo (default Ctrl+F11).
- `WithResumeHotkeyDisabled()` — disable resume listener.
- `OnStop(func(reason string))` — callback on abort.
- `OnPause(func(reason string))` — callback on pause.
- `OnResume(func(reason string))` — callback on resume.
- `WithDriftThresholdPx(int)` — cursor drift tolerance (default 5 px).

### Platform support
- **Windows:** Full native support (focus, scroll, click, type, drift guard, IME).
- **macOS / Linux:** Fallback to CDP input (detectable by anti-bot scripts); no native input, no hotkey listener.

## 7. Dependencies

**Direct:**
- github.com/tuwibu/chromekit v0.2.0

**Indirect (transitive):**
- chromedp (cdproto, chromedp, sysutil)
- gobwas/ws (WebSocket)
- golang.org/x/sys (Windows syscalls)

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
- ✅ Example programs (9 subdirectories)
- ⏳ Test suite (sparse; Phase 06)
- ⏳ Full documentation suite (Phase 08)
