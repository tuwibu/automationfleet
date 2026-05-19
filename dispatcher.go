package chromefleet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tuwibu/chromekit"
)

// Dispatcher owns the priority queue, the single native worker, and the CDP
// worker pool. One native worker is intentional — the OS has one cursor; any
// concurrency at this layer creates cross-window input races.
type Dispatcher struct {
	fleet *Fleet

	mu        sync.Mutex
	cond      *sync.Cond
	queue     priorityQueue
	insertSeq uint64

	started bool
	stopped bool
	paused  bool

	inflight sync.WaitGroup

	cdpJobs chan *queuedJob
	wg      sync.WaitGroup
}

// errCursorDrift signals the OS cursor moved during a critical section, most
// likely because a human grabbed the mouse mid-job. Retried up to
// cfg.driftRetries times with cfg.driftRetryDelay between attempts.
var errCursorDrift = errors.New("chromefleet: cursor drift detected")

func newDispatcher(f *Fleet) *Dispatcher {
	d := &Dispatcher{fleet: f}
	d.cond = sync.NewCond(&d.mu)
	return d
}

// start spins up workers. No-op on second call.
func (d *Dispatcher) start() {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return
	}
	d.started = true
	d.mu.Unlock()

	d.cdpJobs = make(chan *queuedJob, 64)

	d.wg.Add(1)
	go d.nativeWorker()

	for i := 0; i < d.fleet.cfg.cdpWorkers; i++ {
		d.wg.Add(1)
		go d.cdpWorker(i)
	}
}

// enqueue places a job on the priority heap and wakes the native worker.
func (d *Dispatcher) enqueue(j Job) chan JobResult {
	resCh := make(chan JobResult, 1)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		go deliver(resCh, JobResult{
			BrowserID: j.BrowserID,
			Status:    StatusRejected,
			Err:       ErrFleetStopped,
		})
		return resCh
	}
	d.insertSeq++
	qj := &queuedJob{
		id:        nextJobID(),
		insertSeq: d.insertSeq,
		job:       j,
		result:    resCh,
		enqueued:  time.Now(),
	}
	d.queue.push(qj)
	d.inflight.Add(1)
	d.cond.Signal()
	return resCh
}

// abortAll cancels every in-flight job and rejects pending ones.
func (d *Dispatcher) abortAll(reason string) {
	d.mu.Lock()
	d.stopped = true
	pending := d.queue.drain()
	d.cond.Broadcast()
	d.mu.Unlock()

	for _, qj := range pending {
		deliver(qj.result, JobResult{
			ID:        qj.id,
			BrowserID: qj.job.BrowserID,
			Status:    StatusCancelled,
			Err:       fmt.Errorf("aborted: %s", reason),
		})
		d.inflight.Done()
	}
	if d.cdpJobs != nil {
		close(d.cdpJobs)
		d.cdpJobs = nil
	}
}

// waitDrained blocks until in-flight + queue both reach zero.
func (d *Dispatcher) waitDrained() { d.inflight.Wait() }

// pause flips the paused flag. Returns true on the transition from running
// to paused (so callers can fire OnPause exactly once).
func (d *Dispatcher) pause() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.paused || d.stopped {
		return false
	}
	d.paused = true
	return true
}

// resume clears the paused flag and wakes blocked workers. Returns true on
// the transition from paused to running.
func (d *Dispatcher) resume() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.paused || d.stopped {
		return false
	}
	d.paused = false
	d.cond.Broadcast()
	return true
}

// nativeWorker pops jobs in priority order. Native-critical jobs execute
// here serially; CDP-only jobs are forwarded to the cdp pool.
func (d *Dispatcher) nativeWorker() {
	defer d.wg.Done()
	for {
		d.mu.Lock()
		// Wait while: queue empty OR paused (and not stopped).
		for !d.stopped && (d.queue.Len() == 0 || d.paused) {
			d.cond.Wait()
		}
		if d.stopped && d.queue.Len() == 0 {
			d.mu.Unlock()
			return
		}
		qj := d.queue.pop()
		d.mu.Unlock()

		if qj == nil {
			continue
		}

		// Route based on action class AND handle backend. Click/Type/Navigate
		// only need the native critical section when the handle is launched
		// with native backend — CDP-backed handles run those parallel via
		// the cdp pool (no OS cursor → no drift check needed).
		handle := d.fleet.handle(qj.job.BrowserID)
		if needsNativeCriticalSection(qj.job.Action) && handle != nil && handle.Native {
			d.runJob(qj, true)
		} else {
			select {
			case d.cdpJobs <- qj:
			case <-d.fleet.ctx.Done():
				d.completeJob(qj, JobResult{Status: StatusCancelled, Err: d.fleet.ctx.Err()})
			}
		}
	}
}

// cdpWorker consumes parallel-safe jobs.
func (d *Dispatcher) cdpWorker(_ int) {
	defer d.wg.Done()
	for qj := range d.cdpJobs {
		d.runJob(qj, false)
	}
}

// runJob executes a single job under a per-job timeout context.
func (d *Dispatcher) runJob(qj *queuedJob, native bool) {
	ctx, cancel := context.WithTimeout(d.fleet.ctx, qj.job.Timeout)
	defer cancel()

	start := time.Now()
	handle := d.fleet.handle(qj.job.BrowserID)
	if handle == nil {
		d.completeJob(qj, JobResult{Status: StatusFailed, Err: ErrUnknownBrowser, Took: time.Since(start)})
		return
	}

	var err error
	if native {
		err = d.executeCriticalWithRetry(ctx, handle, qj.job.Action)
	} else {
		err = d.executeCDPOnly(ctx, handle, qj.job.Action)
	}

	status := StatusDone
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = StatusCancelled
		} else {
			status = StatusFailed
		}
	}
	took := time.Since(start)
	d.fleet.log.Infof("chromefleet: job=%d browser=%s action=%s status=%s took=%s err=%v",
		qj.id, qj.job.BrowserID, qj.job.Action.kind(), status, took, err)
	d.completeJob(qj, JobResult{Status: status, Err: err, Took: took})
}

// executeCriticalWithRetry retries up to cfg.driftRetries times on cursor
// drift, sleeping cfg.driftRetryDelay between attempts to let a human nudging
// the mouse settle. Non-drift errors surface immediately. Total attempts =
// 1 (initial) + driftRetries.
func (d *Dispatcher) executeCriticalWithRetry(ctx context.Context, h *BrowserHandle, a Action) error {
	err := executeCritical(ctx, d.fleet, h, a)
	for attempt := 1; attempt <= d.fleet.cfg.driftRetries && errors.Is(err, errCursorDrift); attempt++ {
		d.fleet.log.Warnf("chromefleet: cursor drift on browser=%s action=%s — retry %d/%d", h.ID, a.kind(), attempt, d.fleet.cfg.driftRetries)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d.fleet.cfg.driftRetryDelay):
		}
		err = executeCritical(ctx, d.fleet, h, a)
	}
	return err
}

// executeCDPOnly handles parallel-safe actions through CDP — Scroll, Wait,
// plus Click/Type/Navigate when the handle is CDP-backed (Native=false).
// CDP click/type drives Chromium's internal input pipeline, not the OS
// cursor/keyboard — no drift check needed, multiple browsers run in
// parallel.
func (d *Dispatcher) executeCDPOnly(ctx context.Context, h *BrowserHandle, a Action) error {
	page := h.Browser.Current()
	if page == nil {
		return errors.New("chromefleet: no active page")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch act := a.(type) {
	case ScrollAction:
		return scrollViewport(ctx, page, act)
	case WaitAction:
		_, qerr := page.QuerySelector(act.Selector, act.Timeout)
		return qerr
	case ClickAction:
		// Mouse().Click = bezier glide + dwell + click. page.Click =
		// chromedp.Click instant teleport → anti-bot detect.
		return page.Mouse().Click(act.Selector)
	case TypeAction:
		// FocusElement: bezier glide → click → verify activeElement.
		// Sau đó TypeHuman gõ với 80-220ms/char + 5% typo.
		if err := page.Mouse().FocusElement(act.Selector); err != nil {
			return err
		}
		if act.ClearFirst {
			if err := page.Keyboard().ClearInput(); err != nil {
				return err
			}
		}
		return page.Keyboard().TypeHuman(act.Text)
	case NavigateAction:
		timeout := act.Timeout
		if timeout <= 0 {
			timeout = d.fleet.cfg.defaultTimeout
		}
		return page.Navigate(act.URL, timeout)
	default:
		return fmt.Errorf("chromefleet: cdp path got unexpected action %s", a.kind())
	}
}

// scrollViewport scrolls either the viewport or a specific element.
func scrollViewport(_ context.Context, page *chromekit.Page, a ScrollAction) error {
	if a.Selector != "" {
		expr := fmt.Sprintf(`document.querySelector(%q)?.scrollBy(0, %f)`, a.Selector, a.DeltaY)
		return page.Evaluate(expr, nil)
	}
	expr := fmt.Sprintf(`window.scrollBy(0, %f)`, a.DeltaY)
	return page.Evaluate(expr, nil)
}

func (d *Dispatcher) completeJob(qj *queuedJob, r JobResult) {
	r.ID = qj.id
	r.BrowserID = qj.job.BrowserID
	if r.Took == 0 {
		r.Took = time.Since(qj.enqueued)
	}
	deliver(qj.result, r)
	d.inflight.Done()
}

func deliver(ch chan JobResult, r JobResult) {
	defer func() { _ = recover() }()
	select {
	case ch <- r:
	default:
	}
	close(ch)
}
