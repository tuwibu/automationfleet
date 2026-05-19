# Project Changelog

All notable changes to chromefleet are documented here. Format: date, version, summary.

---

## [Unreleased] — Phase 06–08 (Testing, Hardening, Docs)

### Planned
- **Phase 06:** Test coverage expansion from <10% to >80%.
  - Unit tests: priority queue, action validation, hotkey parsing, job lifecycle.
  - Integration tests: multi-browser submission, pause/resume, abort.
  - Windows-specific tests: cursor drift guard, IME guard, hotkey listener.
  - Target completion: 2026-06-30.

- **Phase 07:** Stability hardening.
  - Timeout semantics refinement.
  - Retry logic for transient failures.
  - Enhanced logging + metrics hooks.
  - Target completion: 2026-07-31.

- **Phase 08:** Documentation + examples.
  - API reference (Godoc + guide).
  - Architecture diagrams (Mermaid).
  - Example walkthroughs.
  - Troubleshooting guide.
  - Target completion: 2026-08-31.

---

## [Unreleased] — 2026-05-17 (not yet tagged; latest git tag: v0.2.3)

### Added
- **Per-handle Native routing:** BrowserHandle now has `Native bool` field. When Native=true, actions route through single native worker with drift guard (default 3 retries, 250ms delay). When Native=false (default), actions route through parallel CDP workers with human-like input.
- **CDP human input on parallel path:** Click/Type/Navigate via chromekit Mouse/Keyboard (bezier cursor glide, TypeHuman 80–220ms/char, ClearInput) for anti-bot safety without native OS integration.
- **TypeAction.ClearFirst bool field:** Optional value-clearing via Ctrl+A→Delete before typing, on both native and CDP paths.
- **Configurable drift retry:** New Options: `WithDriftRetries(n)` (default 3), `WithDriftRetryDelay(d)` (default 250ms). Total attempts = 1 + driftRetries.

### Changed
- **Example consolidation:** Removed hotkey_demo, stress_nine, two_browser, nine_navigate, pause_resume_demo, stress_omnibox_click, pid_smoke, omnibox_smoke. Kept five_browser_steps (regression test for Native flag + ClearFirst) and testpage (shared fixture).
- **chromekit upgrade:** v0.2.0 → v0.6.1 (human input implementations).
- **Fleet public API (signatures):**
  - `Submit` now returns `(<-chan JobResult, error)` (was `chan JobResult`).
  - `Pause`/`Resume` take `reason string` arg (was no-arg).
  - `Start`/`Stop` return nothing (was `error`).
- **No public AbortAll():** Replaced by Fleet.Stop() and optional Ctrl+Alt+Shift+S hotkey (disabled by default). Use Fleet.Pause(reason) / Fleet.Resume(reason) for graceful control.
- **Hotkey defaults:** Stop hotkey disabled by default (opt-in via WithStopHotkey). Pause (Ctrl+F10) and Resume (Ctrl+F11) enabled by default.
- **Navigate routing:** No longer always parallel; native-critical when handle.Native=true.
- **BrowserHandle.validate():** Now enforces handle.Native matches Browser.InputBackend().

### Fixed
- Cursor drift retry now respects context cancellation (previously could ignore ctx.Done()).

### Known Limitations
- Test coverage still <10%; critical sections untested.
- Windows-only native path; non-Windows use CDP (detectable by anti-bot).
- Global hotkey listener: Windows-only.

### Dependencies
- github.com/tuwibu/chromekit v0.6.1 (published version, no replace directive).
- Transitive: chromedp/cdproto v0.0.0-20260427, chromedp/chromedp v0.15.1, etc.

### Breaking Changes
- Fleet.Submit signature: returns error as second value (callers must handle).
- Fleet.Pause/Resume now require reason string arg.
- No public AbortAll(); use Fleet.Stop() instead.
- BrowserHandle must include Native field on construction.

---

## [v0.2.0] — 2026-05-10

### Added
- **Complete MVP library:** Fleet orchestrator, dispatcher (queue + critical section), hotkey listener.
- **Core types:**
  - `Fleet`: Orchestrator with lifecycle (New, Start, Stop, Register, Submit, Pause, Resume, AbortAll).
  - `Job` + `JobResult`: Work unit + execution outcome.
  - `Action` interface: ClickAction, TypeAction, NavigateAction.
  - `BrowserHandle`: Fleet-stable browser binding with screen coordinates.
  - `Hotkey`: Key combo (Modifiers + Key), ParseHotkey string parsing.
  - `JobStatus` enum: Done, Failed, Cancelled, Rejected.
  - `Logger` interface: Pluggable logging (Infof, Warnf, Errorf) + NoopLogger.

- **Dispatcher features:**
  - Priority queue (heap-based, priority desc + FIFO tiebreak).
  - Single native worker (atomic critical section): Browser.Focus → ScrollIntoView → BoundingBox → MouseMove → drift-guard → Click [±Type].
  - N CDP workers (parallel non-native actions: Navigate, Evaluate, Screenshot).
  - Pause/Resume: Graceful blocking after in-flight job completes.
  - AbortAll: Destructive abort (cancel pending, let in-flight finish).

- **Platform-specific code:**
  - Windows: Full native input support (focus, scroll, click, type via user32.dll).
  - Windows: Cursor drift detection (GetCursorPos, retry once on drift).
  - Windows: IME guard (keyboard layout switching for non-Latin typing).
  - Windows: Hotkey listener (RegisterHotkey, global Ctrl+Alt+Shift+S abort, Ctrl+F10 pause, Ctrl+F11 resume).
  - Non-Windows: CDP fallback (no native input guarantees; hotkey listener no-op).
  - Build tags: `//go:build windows` / `//go:build !windows` for clean platform split.

- **Configuration options:**
  - WithLogger, WithDefaultTimeout, WithCDPWorkers.
  - WithStopHotkey, WithPauseHotkey, WithResumeHotkey (+ Disabled variants).
  - OnStop, OnPause, OnResume callbacks.
  - WithDriftThresholdPx (cursor drift tolerance, default 5 px).

- **Example programs (9):**
  - hotkey_demo: Abort flow (Ctrl+Alt+Shift+S).
  - stress_nine: 9 browsers, random jobs, latency profiling.
  - two_browser: Cross-window input isolation.
  - nine_navigate: Concurrent multi-browser navigation.
  - pause_resume_demo: Pause/resume hotkey demo.
  - stress_omnibox_click: Repeated navigation + clicks.
  - pid_smoke, omnibox_smoke: Sanity checks.
  - testpage: Shared test server fixture.

- **Minimal test suite:**
  - dispatcher_queue_test.go: 5 test cases (queue order, drain, hotkey parse).
  - hotkey_test.go: 4 test cases (parse, render).
  - Coverage: <10% of codebase.

- **Documentation:**
  - README.md: Architecture overview, quick start, hotkey config, development notes.
  - docs/project-overview-pdr.md: Feature scope, success metrics, gaps.
  - docs/codebase-summary.md: Module layout, entry points, examples map.
  - docs/code-standards.md: Naming, platform split pattern, error handling, test conventions.
  - docs/system-architecture.md: Component diagrams, critical section flow, pause/resume FSM, hotkey abort path.
  - docs/development-roadmap.md: Current state, Phase 06–08 plan, success metrics.
  - docs/project-changelog.md: This file.

### Known Limitations
- **Test coverage:** <10%; critical sections untested.
- **No integration tests:** Real multi-browser workflows not verified.
- **Windows-only native input:** macOS/Linux use CDP fallback (detectable by anti-bot).
- **Global hotkey listener:** Windows-only; non-Windows is no-op.
- **Sparse logging:** Dispatcher logs once per job; no detailed traces.
- **No metrics:** No built-in latency profiling or error rate tracking.

### Breaking Changes
- None (first release).

### Dependencies
- github.com/tuwibu/chromekit v0.2.0 (local replace for development).
- Transitive: chromedp, sysutil, gobwas/ws, golang.org/x/sys, go-json-experiment/json.
- Go 1.26.2+.

### Next Steps
- Phase 06: Expand test coverage to >80%.
- Phase 07: Stability hardening (timeouts, retry logic, logging).
- Phase 08: Full documentation + examples + troubleshooting guide.

---

## [v0.1.0] — 2026-04-15

### Added (Initial Skeleton)
- Fleet type (stub).
- Dispatcher skeleton (queue, worker goroutines).
- Job + JobResult types.
- Action interface.
- Basic Options pattern.

### Status
- Non-functional; internal development checkpoint.
- Removed in v0.2.0 (replaced by complete implementation).

---

## Glossary

### Terminology
- **Critical section:** Atomic sequence: Browser.Focus → ScrollIntoView → BoundingBox → MouseMove → drift-guard → Click [±Type]. Cannot be split.
- **Drift guard:** Cursor position check after MouseMove; detects human interference, retries once if drift exceeds threshold.
- **IME guard:** Keyboard layout switching (force EN-US during typing, restore after). Prevents IME composition from garbling input.
- **Priority queue:** Heap-based work queue; dequeue order: higher priority first, ties broken by insertion order (FIFO).
- **Pause/Resume:** Graceful blocking; pause waits for in-flight job to complete, then blocks worker on cond; resume unblocks via cond.Broadcast.
- **AbortAll:** Destructive abort; cancel all pending jobs (deliver StatusCancelled), let in-flight finish, stop accepting new jobs.
- **Hotkey listener:** System-level event handler (Windows: RegisterHotkey + GetMessage loop; non-Windows: no-op).
- **CDP worker pool:** N parallel goroutines for non-native actions (Navigate, Evaluate, Screenshot, etc.).

### Abbreviations
- **MVP:** Minimum Viable Product.
- **CDP:** Chrome DevTools Protocol.
- **IME:** Input Method Editor (for non-Latin text).
- **HWND:** Handle to a Window (Windows API).
- **RWMutex:** Reader-Writer Mutex (allows concurrent reads, exclusive write).
- **FIFO:** First-In-First-Out.
- **LOC:** Lines of Code.

---

## Document History

| Date | Author | Change |
|------|--------|--------|
| 2026-05-10 | Initial docs | Created docs suite (PDR, codebase summary, code standards, architecture, roadmap, changelog). |

---

## How to Contribute

This changelog is updated at:
- **Release time:** New version block with breaking changes, additions, known issues.
- **Phase completion:** Interim updates (Planned → Planned → Added when shipped).

### Changelog Format
```markdown
## [vX.Y.Z] — YYYY-MM-DD

### Added
- New features (user-facing).

### Changed
- Breaking changes, API rewrites.

### Fixed
- Bug fixes, edge case handling.

### Deprecated
- Features marked for removal.

### Known Issues
- Unresolved bugs, limitations.
```

### Version Scheme
- **v0.x.y:** Pre-release (rapid iteration, breaking changes possible).
- **v1.0.0+:** Stable (semantic versioning, breaking changes rare + documented).

---

## Links

- **GitHub:** [chromefleet](https://github.com/tuwibu/chromefleet)
- **Dependency:** [chromekit](../chromekit)
- **Go Package:** pkg.go.dev/github.com/tuwibu/chromefleet (post-v1.0.0)
- **Docs:** See adjacent markdown files in `docs/`.
