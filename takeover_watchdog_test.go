package automationfleet

import (
	"sync/atomic"
	"testing"
	"time"
)

// waitFor polls cond up to timeout, failing the test if it never holds. Keeps
// the timer-driven watchdog tests free of fixed sleeps / flakiness.
func waitFor(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", msg)
}

// TestTakeoverWatchdogPauseThenAutoResume: a single real move pauses once;
// after the idle window with no further move, it auto-resumes once.
func TestTakeoverWatchdogPauseThenAutoResume(t *testing.T) {
	var pauses, resumes int32
	w := newTakeoverWatchdog(30*time.Millisecond,
		func() { atomic.AddInt32(&pauses, 1) },
		func() { atomic.AddInt32(&resumes, 1) })
	w.start()
	defer w.stop()

	w.notify()
	waitFor(t, time.Second, "pause after real move", func() bool {
		return atomic.LoadInt32(&pauses) == 1
	})
	if got := atomic.LoadInt32(&resumes); got != 0 {
		t.Fatalf("resumed too early: resumes=%d, want 0", got)
	}
	waitFor(t, time.Second, "auto-resume after idle", func() bool {
		return atomic.LoadInt32(&resumes) == 1
	})
	if got := atomic.LoadInt32(&pauses); got != 1 {
		t.Fatalf("pause fired more than once: pauses=%d, want 1", got)
	}
}

// TestTakeoverWatchdogDebounce: continuous real moves keep the fleet paused
// (no resume) until moves stop for a full idle window. Pause fires exactly once.
func TestTakeoverWatchdogDebounce(t *testing.T) {
	var pauses, resumes int32
	idle := 40 * time.Millisecond
	w := newTakeoverWatchdog(idle,
		func() { atomic.AddInt32(&pauses, 1) },
		func() { atomic.AddInt32(&resumes, 1) })
	w.start()
	defer w.stop()

	// Feed moves every ~10ms for ~5 idle-windows worth of time.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			w.notify()
			time.Sleep(idle / 4)
		}
	}()
	<-done

	if got := atomic.LoadInt32(&resumes); got != 0 {
		t.Fatalf("resumed while user still active: resumes=%d, want 0", got)
	}
	if got := atomic.LoadInt32(&pauses); got != 1 {
		t.Fatalf("pause not debounced to once: pauses=%d, want 1", got)
	}
	// After moves stop, it should auto-resume.
	waitFor(t, time.Second, "auto-resume after activity stops", func() bool {
		return atomic.LoadInt32(&resumes) == 1
	})
}

// TestTakeoverWatchdogStopWhilePausedResumes: stopping the watchdog while it
// holds the fleet paused must clear the pause (fire resume) so a later
// re-register does not inherit a stale "user-takeover" reason.
func TestTakeoverWatchdogStopWhilePausedResumes(t *testing.T) {
	var pauses, resumes int32
	w := newTakeoverWatchdog(10*time.Second, // long idle so it won't auto-resume
		func() { atomic.AddInt32(&pauses, 1) },
		func() { atomic.AddInt32(&resumes, 1) })
	w.start()

	w.notify()
	waitFor(t, time.Second, "pause after real move", func() bool {
		return atomic.LoadInt32(&pauses) == 1
	})
	w.stop()
	if got := atomic.LoadInt32(&resumes); got != 1 {
		t.Fatalf("stop while paused did not resume: resumes=%d, want 1", got)
	}
}

// TestTakeoverWatchdogNoMoveNoPause: with no notify, nothing fires.
func TestTakeoverWatchdogNoMoveNoPause(t *testing.T) {
	var pauses, resumes int32
	w := newTakeoverWatchdog(20*time.Millisecond,
		func() { atomic.AddInt32(&pauses, 1) },
		func() { atomic.AddInt32(&resumes, 1) })
	w.start()
	defer w.stop()

	time.Sleep(60 * time.Millisecond)
	if p, r := atomic.LoadInt32(&pauses), atomic.LoadInt32(&resumes); p != 0 || r != 0 {
		t.Fatalf("watchdog fired without any move: pauses=%d resumes=%d, want 0/0", p, r)
	}
}
