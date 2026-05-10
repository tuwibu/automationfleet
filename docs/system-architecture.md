# System Architecture

## High-Level Overview

Chromefleet is an orchestrator layer that multiplexes N Chrome instances over a single serialized native input worker. The critical insight: **the OS has one mouse cursor**. Allowing concurrent threads to issue mouse + keyboard commands creates races (typing while unfocused, clicking while cursor drifts, etc.). The solution is a single native worker thread that executes a complete action atomically.

```
┌─────────────────────────────────────────────────────────────────┐
│ Your Application                                                │
│  - Submit(Job) with BrowserID, Action, Priority                │
└────────────────────────┬────────────────────────────────────────┘
                         │
                    resCh ← JobResult
                         │
┌────────────────────────▼────────────────────────────────────────┐
│ Fleet (Orchestrator)                                            │
│  ├─ handles: map[string]*BrowserHandle (registered browsers)    │
│  ├─ mu: RWMutex (guards handles, stopped state)                │
│  └─ dispatcher: *Dispatcher                                     │
└─────────────────────────────────────────────────────────────────┘
                         │
                    Submit()
                         │
┌────────────────────────▼────────────────────────────────────────┐
│ Dispatcher (Worker Loop)                                        │
│  ├─ queue: priorityQueue (heap-based, priority desc + FIFO)    │
│  ├─ mu: Mutex (guards queue, paused, insertSeq)               │
│  ├─ cond: *sync.Cond (pause/resume, queue-wake)               │
│  ├─ nativeWorker: goroutine (1 thread, critical section)      │
│  ├─ cdpWorkers: N goroutines (parallel, non-native ops)       │
│  └─ hotkeyListener: goroutine (registers hotkeys, fires abort) │
└─────────────────────────────────────────────────────────────────┘
         │                                │                    │
         ├─ ClickAction / TypeAction      ├─ NavigateAction   ├─ Hotkey events
         │                                │                    │
         ▼                                ▼                    ▼
    [CRITICAL SECTION]              [CDP WORKERS]         [HOTKEY LISTENER]
    (atomic)                        (parallel)              (system-level)
     • Focus                        • Evaluate              • Ctrl+Alt+Shift+S
     • ScrollIntoView              • Screenshot               → AbortAll
     • BoundingBox                 • WaitForNav
     • MouseMove                   • etc.
     • Drift-guard
     • Click [±Type]
     • IME-guard
         │
         ▼
    chromekit.Browser (per-browser library)
         │
         ▼
    Chrome DevTools Protocol (CDP)
         │
         ▼
    Chrome Instance
```

## Component Breakdown

### 1. Fleet (Root Orchestrator)

**Responsibilities:**
- Lifecycle: Start/Stop dispatcher and hotkey listener.
- Browser registration: Maintain map of BrowserID → BrowserHandle.
- Job submission: Validate and enqueue jobs to dispatcher.
- Pause/Resume: Signal dispatcher's conditional.
- Abort: Call dispatcher.AbortAll().

**Concurrency model:**
- RWMutex protects handles, stopped flag.
- Submit() acquires write-lock briefly for enqueue validation, releases before returning.

**State:**
```go
type Fleet struct {
    cfg       *config          // immutable after New
    log       Logger           // immutable
    ctx       context.Context  // cancelled on Stop
    cancel    context.CancelFunc
    mu        sync.RWMutex     // guards below
    handles   map[string]*BrowserHandle
    stopped   bool
    stopOnce  sync.Once        // idempotent Stop
    dispatcher *Dispatcher     // created in Start
    hotkeyDone chan struct{}   // hotkey listener teardown signal
}
```

**Public API:**
- `New(opts ...Option) *Fleet` — constructor.
- `Register(h *BrowserHandle) error` — add browser.
- `Start() error` — spin up workers.
- `Stop() error` — teardown, wait for workers.
- `Submit(j Job) (chan JobResult, error)` — enqueue work.
- `Wait() error` — block until Stop is called.
- `Pause()` — signal dispatcher to block after current job.
- `Resume()` — unblock dispatcher.
- `AbortAll()` — cancel in-flight + pending.

### 2. Dispatcher (Worker Orchestrator)

**Responsibilities:**
- Queue management: Maintain priority heap, wake native worker.
- Native worker loop: Dequeue, dispatch to critical section, deliver result.
- CDP worker pool: Parallel non-native action handlers.
- Pause/Resume: Conditional blocking on queue checkout.
- Hotkey abort: Cancel pending jobs, let in-flight finish.

**Concurrency model:**
- Mutex + Cond for queue access and pause/resume signaling.
- Native worker is single-threaded (serializes all native input).
- CDP workers are pool of N goroutines.
- Hotkey listener is separate goroutine (system-level event handler).

**State:**
```go
type Dispatcher struct {
    fleet     *Fleet           // reference to parent
    mu        sync.Mutex       // guards below
    cond      *sync.Cond       // signals queue-wake, pause-resume
    queue     priorityQueue    // heap-based
    insertSeq uint64           // FIFO tiebreak
    started   bool
    stopped   bool
    paused    bool             // dispatcher blocked on cond.Wait
    inflight  sync.WaitGroup   // tracks in-flight native jobs
    cdpJobs   chan *queuedJob  // work queue to CDP workers
    wg        sync.WaitGroup   // all workers
}
```

**Queue Structure (priorityQueue):**
```
Heap of queuedJob:
  {
    id           JobID
    job          Job (user input: BrowserID, Action, Priority, Timeout)
    insertSeq    uint64 (for FIFO tiebreak)
    resCh        chan JobResult (buffered, cap=1)
  }

Ordering: priority desc, then insertSeq asc
Example: [P=10 seq=1, P=10 seq=2, P=5 seq=3] → dequeue P=10 seq=1 first
```

### 3. Critical Section (Native Worker)

**Execution model:** Single goroutine that atomically executes a complete action against a single browser.

**Flow for ClickAction:**
```
1. Dequeue job from priorityQueue
2. Acquire Fleet.mu read-lock (validate BrowserHandle still exists)
3. Get Browser ptr + screen coords (X, Y, Scale)
4. === CRITICAL SECTION BEGINS ===
5. Browser.Focus() — bring window to foreground
6. Page.ScrollIntoView(Selector) — scroll target into viewport
7. Page.BoundingBox(Selector) — fetch element bbox
8. MouseMove(x, y) — move cursor to center of bbox
9. DriftGuard checkpoint: GetCursorPos() — verify cursor hasn't moved
10. If drift detected: retry once (goto 3)
11. Browser.Click() — native click
12. === CRITICAL SECTION ENDS ===
13. resCh ← JobResult{Status: Done, Took: elapsed}
```

**Flow for TypeAction (extends Click):**
```
1–12. Same as ClickAction
13. IME Guard: GetCurrentKeyboardLayout, ForceENUSLayout
14. Browser.Type(text) — native keyboard input
15. Restore keyboard layout
16. === CRITICAL SECTION ENDS ===
17. resCh ← JobResult{Status: Done, Took: elapsed}
```

**Flow for NavigateAction (non-critical):**
```
1. Dequeue job from priorityQueue
2. Acquire Fleet.mu read-lock
3. Get Browser ptr
4. === NO CRITICAL SECTION ===
5. Browser.Page.Navigate(URL) — async via CDP
6. Browser.Page.WaitForNavigation() — block until navigation completes
7. === END ===
8. resCh ← JobResult{Status: Done, Took: elapsed}
```

**Why NavigateAction is parallel:**
- No native input involved (no mouse/keyboard).
- No focus arbitration required.
- Multiple browsers can navigate concurrently without cross-window risk.

**Drift Guard Design:**
```
Assumption: Human interference is rare (devs testing, QA monitoring).
Threshold: Default 5 px (configurable via WithDriftThresholdPx).

Flow:
1. MouseMove(x, y) → driver issues native move
2. GetCursorPos() → read OS cursor position
3. distance = sqrt((x - curPos.x)^2 + (y - curPos.y)^2)
4. if distance > threshold:
     - log.Warnf("cursor drift detected; retrying")
     - retries++
     - if retries < 2: goto step 1 (retry once)
     - else: return errCursorDrift
5. Proceed to Click

Result: Single retry on drift; if drift persists, job fails with StatusFailed.
```

**IME Guard Design (Windows only):**
```
Problem: Non-Latin keyboard layouts (Korean, Chinese, Japanese) intercept
         typed characters, converting to IME candidates. Typing "hello"
         produces garbled IME composition instead of ASCII.

Solution:
1. GetCurrentKeyboardLayout() → save current layout
2. ForceENUSLayout() → switch to English (US)
3. Browser.Type(text) → type safely as ASCII
4. RestoreLayout(saved) → switch back
5. Log warn if restore fails (fallback to manual switch)

On non-Windows: No-op (stub returns unsupported error, caught + logged).
```

### 4. Pause/Resume Semantics

**State machine:**
```
[RUNNING] ──pause()──▶ [PAUSING]
   │                      │
   │                      └─ Wait for in-flight job to complete
   │                      │
   └──────────◀───────── [PAUSED]
   resume()  │  Blocked on cond.Wait

[PAUSED] ──resume()──▶ cond.Broadcast() ──▶ [RUNNING]
```

**Implementation:**
```go
// Pause() in Dispatcher
func (d *Dispatcher) Pause() {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.paused = true
    // Does NOT wait here; in-flight job runs to completion,
    // then nativeWorker() blocks on cond.Wait in next queue checkout
}

// nativeWorker() queue checkout loop
func (d *Dispatcher) nativeWorker() {
    for {
        d.mu.Lock()
        for d.queue.Len() == 0 || d.paused {
            d.cond.Wait() // releases mu, blocks until Signal/Broadcast
        }
        // At this point: queue non-empty AND !paused
        qj := d.queue.Pop() // heap.Pop
        d.mu.Unlock()

        // Execute job outside lock
        result := d.executeJob(qj)
        // ... deliver result ...
    }
}

// Resume() in Dispatcher
func (d *Dispatcher) Resume() {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.paused = false
    d.cond.Broadcast() // wake native worker + any other waiters
}
```

**Guarantees:**
- Pause is *graceful*: in-flight job completes before pause takes effect.
- Resume is *immediate*: blocks for only the lock acquisition.
- No jobs are lost during pause; they remain in queue.

### 5. Hotkey Abort Path

**State machine:**
```
[RUNNING] ──Ctrl+Alt+Shift+S──▶ [ABORTING]
                                   │
                                   ├─ In-flight job: run to critical-section boundary, finish
                                   ├─ Pending jobs: remove from queue, deliver StatusCancelled
                                   └─▶ [STOPPED]

[STOPPED] – no resume; AbortAll is destructive.
```

**Implementation:**
```go
// AbortAll() in Dispatcher
func (d *Dispatcher) AbortAll() {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.stopped = true

    // Drain queue: deliver StatusCancelled to all pending jobs
    for d.queue.Len() > 0 {
        qj := d.queue.Pop()
        go deliver(qj.resCh, JobResult{
            ID:        qj.id,
            BrowserID: qj.job.BrowserID,
            Status:    StatusCancelled,
            Err:       nil,
        })
    }

    // In-flight job: inflight.Wait() will block until current job completes
    // (Job holds inflight.Add(1) at start, calls inflight.Done() at end)
}

// nativeWorker enqueue check
func (d *Dispatcher) enqueue(j Job) chan JobResult {
    d.mu.Lock()
    defer d.mu.Unlock()
    if d.stopped {
        go deliver(resCh, JobResult{Status: StatusRejected, Err: ErrFleetStopped})
        return resCh
    }
    // ... queue job ...
}
```

**Guarantees:**
- In-flight job completes cleanly (no mid-operation interrupt).
- Pending jobs are cancelled immediately (no queue processing).
- No new jobs accepted after AbortAll (Submit returns StatusRejected).
- Stop() waits for in-flight to finish (inflight.Wait()).

### 6. Hotkey Listener (Windows)

**System integration (Windows user32.dll):**
```go
// RegisterHotkey registers a global hotkey
RegisterHotkey(hWnd uintptr, id int, mods uint32, vk uint32) error
// Calls: user32.RegisterHotKey(hWnd, int32(id), modifiers, virtualKey)

// ListenHotkey blocks and delivers hotkey events
func ListenHotkey(ctx context.Context, hk Hotkey, onFire func() error) error
    // 1. RegisterHotkey (global, for current desktop user)
    // 2. Create message-only window
    // 3. GetMessage loop → WM_HOTKEY → invoke onFire()
    // 4. UnregisterHotkey on cleanup
```

**Multi-listener support:**
```go
// Fleet can register multiple hotkeys (stop, pause, resume)
type HotkeyBinding struct {
    hotkey Hotkey
    onFire func() error
}

// Each hotkey runs its own listener goroutine
go ListenHotkey(ctx, binding.hotkey, binding.onFire)
```

**Non-Windows (macOS/Linux):**
```go
func ListenHotkey(ctx context.Context, hk Hotkey, onFire func() error) error {
    <-ctx.Done() // wait for Stop signal, return immediately (no-op)
    return nil
}
```

**Why global hotkeys require platform-specific code:**
- Windows: RegisterHotkey via user32.dll (system-wide event hook).
- macOS: CGEventTapCreate (Quartz Event Services) + async callback.
- Linux: X11 XGrabKey / Wayland unsupported (no system-wide hotkey API).
- Solution: Disable hotkey listener on non-Windows (users running on macOS/Linux can still use Fleet.Pause/Resume/AbortAll programmatically).

### 7. CDP Worker Pool

**Purpose:** Handle non-native actions (Navigate, Evaluate, Screenshot, WaitForNav) in parallel.

**Design:**
```go
// Each CDP worker pulls from a shared work queue
func (d *Dispatcher) cdpWorker(id int) {
    for qj := range d.cdpJobs {
        result := d.executeCDPAction(qj)
        qj.resCh ← result
    }
}

// nativeWorker routes NavigateAction to CDP queue
case *NavigateAction:
    d.cdpJobs ← qj // non-blocking; queue is unbuffered but drained by N workers
```

**Scaling:**
- Default: 4 CDP workers (configurable via WithCDPWorkers).
- No upper limit enforced; users tune based on their workload.

## Data Flow: Complete Job Submission

```
User calls: resCh := fleet.Submit(Job{BrowserID: "b1", Action: ClickAction{...}, Priority: 5})

[1] Fleet.Submit(j Job)
    ├─ Acquire mu.RLock()
    ├─ Validate j.BrowserID in handles
    ├─ Release mu.RLock()
    ├─ Dispatcher.enqueue(j)
    │  ├─ Acquire mu
    │  ├─ Create queuedJob {id, job, insertSeq, resCh}
    │  ├─ heap.Push(queue, queuedJob)
    │  ├─ cond.Signal() — wake nativeWorker if waiting
    │  ├─ Release mu
    │  └─ Return resCh (buffered, cap=1)
    └─ Return resCh

[2] nativeWorker loop
    ├─ Acquire mu
    ├─ Loop: while queue.Len() == 0 or paused: cond.Wait()
    ├─ heap.Pop(queue) → qj
    ├─ Release mu
    ├─ inflight.Add(1)
    ├─ start := time.Now()
    ├─ result, err := executeAction(qj.job.Action, qj.job.BrowserID, qj.job.Timeout)
    ├─ inflight.Done()
    ├─ resCh ← JobResult{
    │    ID:        qj.id,
    │    BrowserID: qj.job.BrowserID,
    │    Status:    (Done|Failed|Cancelled),
    │    Err:       err,
    │    Took:      time.Since(start),
    │  }
    └─ Loop back to [2]

[3] User reads from resCh
    ├─ result := <-resCh
    ├─ Inspect result.Status, result.Err, result.Took
    └─ Continue...
```

## Critical Invariants

1. **Single native worker:** Only one Browser.Focus/Click/Type/MouseMove at a time across the entire fleet.
2. **Serialized input:** No cross-window input races; all native operations are atomic within the critical section.
3. **Pause graceful:** Current job finishes; paused flag blocks next queue checkout.
4. **Abort destructive:** No resume after AbortAll; in-flight finishes, pending is dropped.
5. **No goroutine leaks:** All workers blocked on context.Done or channel close by Stop().
6. **Result delivery:** Every job gets exactly one result on its resCh (except on queue drain, where it's dropped).

## Performance Characteristics

| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Submit(Job) | O(log N) | heap.Push |
| Queue checkout | O(log N) | heap.Pop |
| Pause/Resume | O(1) | cond.Signal/Broadcast |
| AbortAll | O(N) | drain queue, cancel each |
| Action execute | O(variable) | depends on browser, network, DOM |

## Platform-Specific Branches

| Feature | Windows | macOS/Linux |
|---------|---------|------------|
| Native input | ✅ Full (Focus, scroll, click, type) | ❌ CDP fallback |
| Cursor drift guard | ✅ GetCursorPos via user32.dll | ❌ Skipped |
| IME guard | ✅ Keyboard layout switching | ❌ Skipped |
| Hotkey listener | ✅ RegisterHotkey, WM_HOTKEY | ❌ No-op (no system-wide hook API) |
| Result | Production-grade native input guarantees | Degraded (anti-bot detectable) |

## Design Rationale

**Single native worker:** OS provides one cursor; concurrent mouse commands create logical races. Atomic critical section eliminates this problem at the cost of serialized input latency.

**Priority queue + FIFO tiebreak:** Users can prioritize jobs (e.g., critical click before debug screenshot) while maintaining deterministic ordering for jobs at the same priority.

**Conditional pause/resume:** Automation engineers need to pause mid-workflow (e.g., inspect state, adjust next job), then resume. Conditional is cheaper than stopping/restarting.

**Global hotkey listener:** Provides user-initiated abort (Ctrl+Alt+Shift+S) without code changes. Useful for emergency stops during long-running scripts.

**Platform-specific code via build tags:** One binary, no runtime branching, no dead-code confusion.
