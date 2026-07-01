package automationfleet

import (
	"sync"
	"time"
)

// takeoverIdle is how long the fleet stays auto-paused after the last real
// mouse movement before auto-resuming. The user grabs the cursor → fleet
// pauses → 10s of no further real movement → fleet resumes.
const takeoverIdle = 10 * time.Second

// ReasonUserTakeover is the pause reason the takeover watchdog uses when it
// detects real user mouse activity. Exported so status readers and tests share
// one source of truth instead of duplicating the magic string.
const ReasonUserTakeover = "user-takeover"

// takeoverWatchdog auto-pauses the fleet on real (non-injected) mouse activity
// and auto-resumes after `idle` elapses with no further activity. The pure
// timer/debounce logic lives here (no OS dependency) so it is unit-testable;
// the low-level hook feeds it via notify().
type takeoverWatchdog struct {
	idle   time.Duration
	pause  func()
	resume func()

	// moves is a size-1 coalescing channel: the hook callback drops a token per
	// real move; a full buffer just means "at least one move pending", which is
	// all the run loop needs.
	moves  chan struct{}
	stopCh chan struct{}
	done   chan struct{}

	startOnce sync.Once
	stopOnce  sync.Once
}

func newTakeoverWatchdog(idle time.Duration, pause, resume func()) *takeoverWatchdog {
	return &takeoverWatchdog{
		idle:   idle,
		pause:  pause,
		resume: resume,
		moves:  make(chan struct{}, 1),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// notify signals one real mouse movement. Non-blocking + coalescing — it is
// called from the low-level hook callback, which must never block (a callback
// exceeding LowLevelHooksTimeout is silently dropped by Windows).
func (w *takeoverWatchdog) notify() {
	select {
	case w.moves <- struct{}{}:
	default:
	}
}

// start launches the run loop. Idempotent.
func (w *takeoverWatchdog) start() {
	w.startOnce.Do(func() { go w.run() })
}

// stop tears down the run loop and blocks until it exits. Idempotent. If the
// fleet was auto-paused, stop clears the "user-takeover" reason so a later
// re-register does not inherit a stale pause.
func (w *takeoverWatchdog) stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		<-w.done
	})
}

func (w *takeoverWatchdog) run() {
	defer close(w.done)

	// Timer starts armed then is immediately stopped+drained so the first real
	// move can Reset it from a clean state.
	timer := time.NewTimer(w.idle)
	if !timer.Stop() {
		<-timer.C
	}

	paused := false
	for {
		select {
		case <-w.moves:
			if !paused {
				w.pause()
				paused = true
			}
			// stop-drain-reset: never bare-Reset a timer that may have already
			// fired — drain its channel first to avoid a spurious resume race.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.idle)
		case <-timer.C:
			if paused {
				w.resume()
				paused = false
			}
		case <-w.stopCh:
			timer.Stop()
			if paused {
				w.resume()
			}
			return
		}
	}
}
