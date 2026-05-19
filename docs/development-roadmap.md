# Development Roadmap

**Current Date:** 2026-05-17  
**Project Status:** Unreleased (latest git tag v0.2.3) — Dual-path routing (native + CDP human input), sparse test coverage  
**Next Focus:** Test suite expansion (Phase 06 active), stability hardening, documentation

## Completed Phases

### Phase 01: chromekit Prep
**Status:** ✅ Done (chromekit v0.6.1 — human input implementations)

- Mutex-protected window focus (SetWindowForeground).
- HWND (window handle) support for multi-monitor cursor math.
- Native input backend via user32.dll (click, type, mouse move).
- Browser lifecycle (Connect, Close, Focus).

### Phase 02: chromefleet Bootstrap
**Status:** ✅ Done

- Fleet type (orchestrator, lifecycle).
- Job + JobResult (work unit + outcome).
- Action interface (ClickAction, TypeAction, NavigateAction).
- Config builder (Option pattern).
- BrowserHandle (fleet-stable browser binding).

### Phase 03: Dispatcher (Queue + Critical Section)
**Status:** ✅ Done

- Priority queue (heap-based, priority desc + FIFO tiebreak).
- Native worker (single-threaded critical section).
- CDP worker pool (parallel non-native actions).
- Pause/Resume (conditional blocking on queue checkout).
- Cursor drift guard (GetCursorPos check, retry once).
- IME guard (keyboard layout switching on Windows).

### Phase 04: Hotkey Listener
**Status:** ✅ Done

- ParseHotkey (string → Hotkey struct parsing).
- RegisterHotkey + ListenHotkey (Windows system-level registration).
- Multi-listener support (stop, pause, resume).
- Graceful teardown (UnregisterHotkey on context.Done).
- Non-Windows stubs (safe no-op on macOS/Linux).

### Phase 05: PoC + Examples (v0.2.0)
**Status:** ✅ Done → ✅ Consolidated (2026-05-17, unreleased)

- Archived: hotkey_demo, stress_nine, two_browser, nine_navigate, pause_resume_demo, stress_omnibox_click, pid_smoke, omnibox_smoke.
- **Current:** five_browser_steps (regression test for Native flag + TypeAction.ClearFirst), testpage (shared test server).

---

## Active Phase: Testing & Stability (Phase 06)

**Target completion:** 2026-06-30  
**Effort estimate:** 2–3 weeks (depends on platform test coverage needs)

### Goals
1. **Expand unit test coverage** from <10% to >80%.
2. **Add integration tests** for multi-browser workflows.
3. **Platform-specific tests** for Windows native input + hotkey listener.
4. **Stability hardening:** edge cases, timeout handling, error paths.

### Work Items

#### 6.1: Unit Tests — Priority Queue
- [ ] TestPriorityQueueInsert_Multiple (verify heap structure).
- [ ] TestPriorityQueuePop_Order (higher priority dequeues first).
- [ ] TestPriorityQueueTiebreak_FIFO (same priority → FIFO).
- [ ] TestPriorityQueueDrain_Empty (pop from empty returns nil).
- [ ] TestPriorityQueueLen (accurate count tracking).

**Files:** `dispatcher_queue_test.go` (new tests, ~60 LOC)

#### 6.2: Unit Tests — Action Validation
- [ ] TestClickAction_Validate_RequiresSelector (reject empty Selector).
- [ ] TestTypeAction_Validate_RequiresSelector (reject empty Selector).
- [ ] TestTypeAction_Validate_RequiresText (reject empty Text).
- [ ] TestNavigateAction_Validate_RequiresURL (reject empty URL).
- [ ] TestBrowserHandle_Validate_RequiresID (reject empty ID).
- [ ] TestBrowserHandle_Validate_RequiresBrowser (reject nil Browser).
- [ ] TestBrowserHandle_Validate_ScaleDefault (zero Scale → 1.0).

**Files:** `action_test.go`, `browser_handle_test.go` (new, ~80 LOC)

#### 6.3: Unit Tests — Hotkey Parsing
- [x] TestParseHotkey_Cases (existing, expand coverage).
  - [ ] TestParseHotkey_SingleModifier (just "Ctrl").
  - [ ] TestParseHotkey_NoModifier ("A").
  - [ ] TestParseHotkey_FKey_Lowercase ("ctrl+f10").
  - [ ] TestParseHotkey_InvalidKey (reject unknown key).
  - [ ] TestParseHotkey_DuplicateModifier (handle "Ctrl+Ctrl+A").

**Files:** `hotkey_test.go` (extend existing, ~40 LOC)

#### 6.4: Unit Tests — Job Lifecycle
- [ ] TestJobStatus_String (verify enum string rendering).
- [ ] TestJobID_Sequence (IDs increment, never reused).
- [ ] TestJobResult_Marshaling (serialize/deserialize if needed).

**Files:** `job_test.go` (new, ~30 LOC)

#### 6.5: Integration Tests — Multi-Browser Job Submission
- [ ] TestFleet_Submit_SingleBrowser (submit 5 jobs, verify all execute).
- [ ] TestFleet_Submit_MultiB Browser (submit to 2+ browsers concurrently).
- [ ] TestFleet_Submit_PriorityOrdering (submit high + low priority, verify order).
- [ ] TestFleet_Submit_FIFOTiebreak (same priority → FIFO).
- [ ] TestFleet_Submit_RejectedWhenStopped (Submit after Stop returns StatusRejected).

**Files:** `fleet_integration_test.go` (new, ~150 LOC)

#### 6.6: Integration Tests — Pause/Resume
- [ ] TestFleet_Pause_BlocksQueueCheckout (pause mid-queue, verify next job waits).
- [ ] TestFleet_Resume_UnblocksDispatcher (resume unblocks, next job executes).
- [ ] TestFleet_Pause_AllowsInFlightCompletion (in-flight job finishes before pause takes effect).
- [ ] TestFleet_PauseResume_Cycle (pause → resume → pause → resume works repeatedly).

**Files:** `fleet_integration_test.go` (extend, ~100 LOC)

#### 6.7: Integration Tests — Abort
- [ ] TestFleet_AbortAll_CancelsPending (pending jobs get StatusCancelled).
- [ ] TestFleet_AbortAll_LetInFlightFinish (in-flight job completes cleanly).
- [ ] TestFleet_AbortAll_StopsAcceptingNewJobs (Submit after AbortAll returns StatusRejected).
- [ ] TestFleet_AbortAll_NoResume (AbortAll is destructive, no Resume allowed).

**Files:** `fleet_integration_test.go` (extend, ~120 LOC)

#### 6.8: Windows-Specific Tests
- [ ] TestCursorDriftGuard_NoMovement (GetCursorPos matches expected, no retry).
- [ ] TestCursorDriftGuard_Drift_Retry (cursor moves beyond threshold, job retries).
- [ ] TestCursorDriftGuard_PersistentDrift_Fails (drift persists after retry, job fails).
- [ ] TestIMEGuard_SwitchLayout (force EN-US, type, restore original).
- [ ] TestIMEGuard_NonWindows_Skipped (non-Windows fallback skips guard).

**Files:** `dispatcher_critical_windows_test.go` (new, ~180 LOC, Windows-only build tag)

**Note:** Cursor drift + IME tests require mocking or real Chrome instance + Windows API. Consider:
- Mock `GetCursorPos()` via interface injection (internal/winapi provides stub on non-Windows).
- Real Windows CI (e.g., GitHub Actions on Windows runner) for end-to-end validation.

#### 6.9: Hotkey Listener Tests
- [ ] TestHotkeyListener_RegisterAndFire (hotkey fires callback).
- [ ] TestHotkeyListener_MultipleHotkeys (multiple listeners coexist).
- [ ] TestHotkeyListener_Teardown_UnregistersHotkey (cleanup on context.Done).
- [ ] TestHotkeyListener_NonWindows_NoOp (non-Windows listener returns immediately).

**Files:** `hotkey_listener_test.go` (new, ~120 LOC, Windows/non-Windows variants)

**Complexity note:** System-level hotkey registration requires user32.dll or macOS/Linux equivalents. Tests may require:
- Windows-specific CI setup.
- Mocking SendMessage / GetMessage.
- Careful teardown to avoid leaving global hotkeys registered.

#### 6.10: Timeout Handling Edge Cases
- [ ] TestJob_DefaultTimeout_UsesFleetDefault (zero timeout → fleet default).
- [ ] TestJob_CustomTimeout_Overrides (non-zero timeout → use job timeout).
- [ ] TestJob_TimeoutExceeded_CancelledStatus (job runs longer than timeout → StatusCancelled).
- [ ] TestJob_TimeoutZero_NoLimit (clarify semantics: zero = default or no limit?).

**Files:** `fleet_integration_test.go` (extend, ~80 LOC)

#### 6.11: Error Path Tests
- [ ] TestFleet_Register_ValidatesBrowserHandle (nil Browser → error).
- [ ] TestFleet_Submit_ValidatesAction (nil Action → StatusRejected).
- [ ] TestFleet_Submit_UnknownBrowserID (BrowserID not registered → StatusRejected).
- [ ] TestDispatcher_ExecuteAction_CatchesPanic (action panics → StatusFailed, no crash).
- [ ] TestDispatcher_NativeWorker_RecoverFromError (one job fails, next job runs).

**Files:** `fleet_integration_test.go`, `dispatcher_test.go` (new, ~100 LOC)

#### 6.12: Test Fixtures + Utilities
- [ ] Create mock Logger (captures log messages for inspection).
- [ ] Create test BrowserHandle factory (quick setup for tests).
- [ ] Create test Action factories (ClickAction, TypeAction with defaults).
- [ ] Mock chromekit.Browser (for testing without real Chrome).

**Files:** `internal/testutil/mock.go` (new, ~150 LOC)

### Test Coverage Target
| Component | Current | Target | Notes |
|-----------|---------|--------|-------|
| dispatcher_queue.go | ~30% | 95% | Heap ops well-covered |
| fleet.go | ~5% | 85% | Lifecycle, Register, Submit, Stop |
| dispatcher.go | ~2% | 80% | Worker loop, pause/resume, abort |
| action.go | ~10% | 90% | Validation, dispatch paths |
| hotkey.go | ~40% | 95% | Parse, listener, teardown |
| job.go | ~5% | 90% | Status enum, JobID sequence |
| browser_handle.go | ~0% | 90% | Validation |
| internal/winapi | ~0% | 70% | Windows-specific; mocked on non-Windows |

**Overall target:** >80% coverage (measured by `go test -cover`).

### Success Criteria
- [ ] `go test ./... -cover` reports >80% coverage.
- [ ] All test cases pass on Windows + macOS/Linux (CI green).
- [ ] No panics or goroutine leaks in tests (verify with `-race` flag).
- [ ] Example programs still run without regression.
- [ ] Integration tests demonstrate multi-browser pause/resume/abort workflows.

---

## Planned Phase: Stability Hardening (Phase 07)

**Target start:** 2026-07-01  
**Target completion:** 2026-07-31  
**Effort estimate:** 2–3 weeks

### Goals
1. **Timeout refinement:** Clear semantics, edge case handling.
2. **Retry logic:** Drift guard, transient focus loss.
3. **Logging improvements:** Detailed traces for debugging.
4. **Error recovery:** Graceful degradation on edge cases.

### Work Items
- [ ] Define timeout semantics: zero = fleet default, or no limit?
- [ ] Implement context timeout propagation (Job.Timeout → context.WithTimeout).
- [ ] Add retry logic for transient failures (focus loss, navigation timeout).
- [ ] Enhance dispatcher logs with request ID + state machine transitions.
- [ ] Add metrics/instrumentation hooks (job latency, error rates).
- [ ] Graceful degradation: if Browser.Focus fails, warn + continue (don't crash).
- [ ] Handle browser disconnect mid-job (chromekit.Browser closed → StatusFailed).

---

## Planned Phase: Documentation + Examples (Phase 08)

**Target start:** 2026-08-01  
**Target completion:** 2026-08-31  
**Effort estimate:** 2 weeks

### Goals
1. **Complete API reference** (Godoc + guide).
2. **Architecture diagrams** (Mermaid, system flow).
3. **Example programs with walkthroughs.**
4. **Troubleshooting guide** (common issues, debug techniques).
5. **Integration guide** (how to wire chromefleet into your project).

### Work Items
- [ ] Write Godoc comments for all exported types/functions.
- [ ] Create `docs/api-reference.md` (exported API surface + option descriptions).
- [ ] Create `docs/examples-walkthrough.md` (explain each example program).
- [ ] Create `docs/architecture.md` (worker diagram, critical section flowchart, pause/resume FSM).
- [ ] Create `docs/troubleshooting.md` (hotkey not firing, cursor drift, IME issues).
- [ ] Create `docs/platform-notes.md` (Windows native vs macOS/Linux CDP fallback).
- [ ] Add quick-start example to README.
- [ ] Generate architecture diagrams in Mermaid.

---

## Known Gaps & Deferred Work

| Gap | Impact | Deferred Phase | Notes |
|-----|--------|---|---|
| Test coverage <10% | High — critical sections untested | Phase 06 | Priority work |
| No integration tests | High — real workflows unknown | Phase 06 | Priority work |
| Timeout semantics unclear | Medium — edge cases lurk | Phase 07 | Design work needed |
| Limited logging | Medium — debugging hard | Phase 07 | Instrumentation |
| Sparse documentation | Medium — API surface unclear | Phase 08 | Docs-only |
| No metrics/instrumentation | Low — observability missing | Phase 07+ | Optional, TBD |
| No CI/CD workflows | Low — pure library, no deploy | Never | Out of scope |
| No Dockerfile | Low — per-consumer choice | Never | Out of scope |

---

## Dependency Tree

```
Phase 01 ✅
    │
Phase 02 ✅
    │
Phase 03 ✅
    │
Phase 04 ✅
    │
Phase 05 ✅
    │
Phase 06 (active) ← Testing & Stability
    │
Phase 07 ← Hardening (depends on Phase 06 completion)
    │
Phase 08 ← Documentation (depends on Phase 07 completion)
```

**Critical path:** Phase 06 must complete before Phase 07/08 can start (test suite is a gate for public confidence).

---

## Deployment & Release

**Current versioning:** v0.2.0 (Pre-release, pending test suite)

### Release Criteria for v1.0.0
- [ ] Phase 06 complete (>80% test coverage).
- [ ] Phase 07 complete (stability hardening + logging).
- [ ] Phase 08 complete (full documentation).
- [ ] No open issues in GitHub.
- [ ] Multi-browser stress tests pass (hotkey_demo, stress_nine, etc.).
- [ ] Cross-platform verification (Windows + macOS + Linux CI green).

### Release Process (Post-v1.0.0)
1. Merge all phases into `main`.
2. Create git tag `v1.0.0`.
3. Push to GitHub (chromefleet repo, remove `replace` directive in go.mod).
4. Publish to pkg.go.dev.
5. Create release notes summarizing features + breaking changes.

---

## Success Metrics (EoD Phase 08)

| Metric | Target | Notes |
|--------|--------|-------|
| Test coverage | >80% | go test -cover |
| Example programs | 9 fully working + documented | stress_nine, hotkey_demo, etc. |
| Documentation | >95% exported symbols documented | Godoc + guides |
| Zero open bugs | Yes | Tracked in GitHub Issues |
| Platform support | Windows (full) + macOS/Linux (fallback) | Native on Windows, CDP-only otherwise |
| Release artifact | chromefleet v1.0.0 on pkg.go.dev | Public GA |

---

## Archive: Decisions + Rationale

### Why single native worker (not per-browser)?
- OS has one cursor; N workers = N potential races (cross-window typing, drift).
- Single worker eliminates race conditions entirely.
- Cost: Serialized native input latency (acceptable for automation).

### Why priority queue + FIFO tiebreak?
- Users need control: "prioritize this critical click over debug screenshots."
- FIFO tiebreak provides determinism: same-priority jobs execute in order.
- Heap is O(log N); efficient for 100s of jobs.

### Why conditional for pause/resume (not restart)?
- Graceful: in-flight job finishes before pause takes effect.
- Efficient: O(1) wake/block vs teardown/restart overhead.
- User-friendly: pause mid-workflow, inspect, resume without losing state.

### Why destructive AbortAll (no resume)?
- Abort semantics are: "stop everything, now."
- Resume after abort would be confusing (which jobs? what state?).
- Design: use Pause/Resume for temporary holds, AbortAll only for emergencies.

### Why platform-specific build tags (not runtime checks)?
- One binary = one platform code path; no dead-code confusion.
- Cleaner than `if runtime.GOOS == "windows"` scattered throughout.
- Matches Go ecosystem conventions (e.g., Go stdlib uses build tags for platform code).

### Why chatty logging (once per job)?
- Helps debugging multi-browser workflows.
- Job ID enables traceability across log lines.
- Allocation-light: pluggable Logger interface (no forced dependency).

---

## Quick Links
- **Code:** [GitHub chromefleet](https://github.com/tuwibu/chromefleet)
- **Dependency:** [chromekit](../chromekit)
- **Examples:** `examples/` subdirectory
- **Docs:** `docs/` (you are here)
