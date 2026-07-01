package automationfleet

import "testing"

// TestDispatcherPauseReasonSet verifies the multi-reason pause semantics: the
// dispatcher stays paused while any reason is active, and only the empty→non-empty
// / non-empty→empty transitions return true (so OnPause/OnResume fire once).
func TestDispatcherPauseReasonSet(t *testing.T) {
	d := newDispatcher(nil)

	if got := d.pause("ui"); !got {
		t.Fatalf("first pause: transition = %v, want true", got)
	}
	if got := d.pause("user-takeover"); got {
		t.Fatalf("second pause (different reason): transition = %v, want false", got)
	}
	if !d.isPaused() {
		t.Fatal("isPaused = false after two pauses, want true")
	}

	// Resuming one of two reasons keeps the dispatcher paused.
	if got := d.resume("user-takeover"); got {
		t.Fatalf("resume 1 of 2: transition = %v, want false", got)
	}
	if !d.isPaused() {
		t.Fatal("isPaused = false while 'ui' still held, want true")
	}

	// Resuming the last reason transitions to running.
	if got := d.resume("ui"); !got {
		t.Fatalf("resume last reason: transition = %v, want true", got)
	}
	if d.isPaused() {
		t.Fatal("isPaused = true after all reasons cleared, want false")
	}
}

// TestDispatcherPauseIdempotent verifies same-reason idempotency (mirrors the
// existing bool contract: pausing/resuming the same reason twice fires once).
func TestDispatcherPauseIdempotent(t *testing.T) {
	d := newDispatcher(nil)

	if got := d.pause("test"); !got {
		t.Fatalf("first pause: %v, want true", got)
	}
	if got := d.pause("test"); got {
		t.Fatalf("duplicate pause(same reason): %v, want false", got)
	}
	if got := d.resume("absent"); got {
		t.Fatalf("resume(absent reason): %v, want false", got)
	}
	if got := d.resume("test"); !got {
		t.Fatalf("resume held reason: %v, want true", got)
	}
	if got := d.resume("test"); got {
		t.Fatalf("duplicate resume: %v, want false", got)
	}
}

// TestDispatcherPauseStoppedNoop verifies stopped dispatcher rejects pause/resume.
func TestDispatcherPauseStoppedNoop(t *testing.T) {
	d := newDispatcher(nil)
	d.stopped = true
	if d.pause("ui") {
		t.Fatal("pause on stopped dispatcher returned true, want false")
	}
	if d.resume("ui") {
		t.Fatal("resume on stopped dispatcher returned true, want false")
	}
}
