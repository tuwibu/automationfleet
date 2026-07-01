package automationfleet

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tuwibu/automationfleet/internal/winapi"
	"github.com/tuwibu/chromekit"
	"github.com/tuwibu/firefoxkit"
)

// Logger is what automationfleet writes diagnostic lines to. Wire your own (zap,
// zerolog, log/slog) or use NoopLogger. Keep it allocation-light — the
// dispatcher logs once per job.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// NoopLogger silently drops every line.
type NoopLogger struct{}

func (NoopLogger) Infof(string, ...any)  {}
func (NoopLogger) Warnf(string, ...any)  {}
func (NoopLogger) Errorf(string, ...any) {}

// Option configures New.
type Option func(*config)

type config struct {
	logger         Logger
	defaultTimeout time.Duration
	cdpWorkers     int

	// Stop hotkey aborts every in-flight + pending job. Default DISABLED —
	// users who want abort semantics opt-in via WithStopHotkey.
	stopHotkey    Hotkey
	stopEnabled   bool
	onStop        func(reason string)

	// Pause/Resume hotkey pair. Default Ctrl+F10 / Ctrl+F11. Pause drains
	// in-flight, then blocks the worker on a cond — resume unblocks.
	pauseHotkey   Hotkey
	pauseEnabled  bool
	onPause       func(reason string)
	resumeHotkey  Hotkey
	resumeEnabled bool
	onResume      func(reason string)

	driftThreshold int

	// driftRetries: số lần retry trên errCursorDrift (initial attempt KHÔNG
	// tính). Default 3 cho phép user nudge chuột vài lần mà không fall back.
	// driftRetryDelay: chờ giữa các attempt để cursor settle.
	driftRetries    int
	driftRetryDelay time.Duration
}

// DefaultPauseHotkey is Ctrl+F10. F-keys + Ctrl rarely conflict with global
// shortcuts on Windows / VS Code / Chrome.
var DefaultPauseHotkey = Hotkey{Mods: ModCtrl, Key: KeyF10}

// DefaultResumeHotkey is Ctrl+F11.
var DefaultResumeHotkey = Hotkey{Mods: ModCtrl, Key: KeyF11}

func defaultConfig() *config {
	return &config{
		logger:         NoopLogger{},
		defaultTimeout: 10 * time.Second,
		cdpWorkers:     4,

		stopHotkey:  DefaultStopHotkey,
		stopEnabled: false, // opt-in: stop is destructive (no resume)

		pauseHotkey:   DefaultPauseHotkey,
		pauseEnabled:  true,
		resumeHotkey:  DefaultResumeHotkey,
		resumeEnabled: true,

		driftThreshold:  5,
		driftRetries:    3,
		driftRetryDelay: 250 * time.Millisecond,
	}
}

func WithLogger(l Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

func WithDefaultTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.defaultTimeout = d
		}
	}
}

func WithCDPWorkers(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.cdpWorkers = n
		}
	}
}

// WithStopHotkey enables the abort combo (default Ctrl+Alt+Shift+S) and
// overrides the key. Stop is destructive — fires AbortAll, no resume.
func WithStopHotkey(h Hotkey) Option {
	return func(c *config) {
		c.stopHotkey = h
		c.stopEnabled = true
	}
}

// WithStopHotkeyDisabled is now a no-op (stop is disabled by default).
// Kept for backwards-compat with existing examples.
func WithStopHotkeyDisabled() Option {
	return func(c *config) { c.stopEnabled = false }
}

// WithPauseHotkey overrides the default Ctrl+F10 pause combo.
func WithPauseHotkey(h Hotkey) Option {
	return func(c *config) {
		c.pauseHotkey = h
		c.pauseEnabled = true
	}
}

// WithPauseHotkeyDisabled disables the pause hotkey listener.
func WithPauseHotkeyDisabled() Option {
	return func(c *config) { c.pauseEnabled = false }
}

// WithResumeHotkey overrides the default Ctrl+F11 resume combo.
func WithResumeHotkey(h Hotkey) Option {
	return func(c *config) {
		c.resumeHotkey = h
		c.resumeEnabled = true
	}
}

// WithResumeHotkeyDisabled disables the resume hotkey listener.
func WithResumeHotkeyDisabled() Option {
	return func(c *config) { c.resumeEnabled = false }
}

// OnStop registers a callback fired when AbortAll runs (hotkey, manual, or
// fleet shutdown).
func OnStop(cb func(reason string)) Option {
	return func(c *config) { c.onStop = cb }
}

// OnPause registers a callback fired when the pause hotkey hits or Pause()
// is called. The callback runs on the listener goroutine — keep it short.
func OnPause(cb func(reason string)) Option {
	return func(c *config) { c.onPause = cb }
}

// OnResume registers a callback fired when the resume hotkey hits or
// Resume() is called.
func OnResume(cb func(reason string)) Option {
	return func(c *config) { c.onResume = cb }
}

// WithDriftThresholdPx tunes the cursor-drift guard. If the OS cursor moves
// further than this between MoveTo and the next op, the dispatcher assumes
// human interference and retries the job.
func WithDriftThresholdPx(px int) Option {
	return func(c *config) {
		if px > 0 {
			c.driftThreshold = px
		}
	}
}

// WithDriftRetries sets max retries on cursor drift (default 3). Initial
// attempt KHÔNG tính — total = 1 + n. Set 0 để disable retry (legacy single-shot).
func WithDriftRetries(n int) Option {
	return func(c *config) {
		if n >= 0 {
			c.driftRetries = n
		}
	}
}

// WithDriftRetryDelay set thời gian chờ giữa 2 attempt drift retry (default
// 250ms). Cho cursor settle khi user đang nudge.
func WithDriftRetryDelay(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.driftRetryDelay = d
		}
	}
}

// Fleet is the public face of the orchestrator. Construct with New, register
// browsers, Submit jobs, Stop when done.
type Fleet struct {
	cfg    *config
	log    Logger
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.RWMutex
	handles     map[string]*BrowserHandle
	nativeCount int // # of registered handles with Native=true; gates the takeover hook
	stopped     bool
	stopOnce    sync.Once

	dispatcher *Dispatcher
	hotkeyDone chan struct{}

	// Mouse-takeover watchdog, installed only while ≥1 native browser is
	// registered. takeoverMu serializes install/uninstall independently of mu so
	// the OS hook syscalls never run under the registry lock.
	takeoverMu    sync.Mutex
	watchdog      *takeoverWatchdog
	hookUninstall func()
}

// New builds a Fleet. Call Start() before Submit.
func New(opts ...Option) *Fleet {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &Fleet{
		cfg:     cfg,
		log:     cfg.logger,
		ctx:     ctx,
		cancel:  cancel,
		handles: make(map[string]*BrowserHandle),
	}
	f.dispatcher = newDispatcher(f)
	return f
}

// Register adds a browser handle to the fleet. Must be called before Submit
// references the same BrowserID.
func (f *Fleet) Register(h *BrowserHandle) error {
	if err := h.validate(); err != nil {
		return err
	}
	f.mu.Lock()
	if _, exists := f.handles[h.ID]; exists {
		f.mu.Unlock()
		return errors.New("automationfleet: browser id already registered: " + h.ID)
	}
	f.handles[h.ID] = h
	if h.Native {
		f.nativeCount++
	}
	f.mu.Unlock()

	f.reconcileTakeover()
	return nil
}

// RegisterChrome wraps a chromekit.Browser in a Driver and registers it under
// id. native must match how the browser was launched (BackendNative vs CDP);
// x/y/scale are the native window origin + DPI scale used by the drift guard
// (ignored when native is false).
func (f *Fleet) RegisterChrome(id string, b *chromekit.Browser, native bool, x, y int, scale float64) error {
	if b == nil {
		return errors.New("automationfleet: RegisterChrome: browser is nil")
	}
	return f.Register(&BrowserHandle{
		ID: id, Driver: WrapChrome(b), Native: native, X: x, Y: y, Scale: scale,
	})
}

// RegisterFirefox wraps a firefoxkit.Browser in a Driver and registers it under
// id. Prefer native=false (BiDi/Remote) — native firefox input has a
// content-offset gap in firefoxkit (see WrapFirefox). x/y/scale are the native
// window origin + DPI scale used by the drift guard (ignored when native is false).
func (f *Fleet) RegisterFirefox(id string, b *firefoxkit.Browser, native bool, x, y int, scale float64) error {
	if b == nil {
		return errors.New("automationfleet: RegisterFirefox: browser is nil")
	}
	return f.Register(&BrowserHandle{
		ID: id, Driver: WrapFirefox(b), Native: native, X: x, Y: y, Scale: scale,
	})
}

// Unregister removes a browser handle from the fleet. Returns ErrUnknownBrowser
// if the ID was never registered. Pending jobs targeting this BrowserID will
// fail with ErrUnknownBrowser when dispatched (dispatcher checks handle nil
// before executing). Symmetric to Register — required so callers can re-Register
// the same ID after the underlying Browser closes; without it the internal
// registry holds a stale handle forever and the next Register rejects.
func (f *Fleet) Unregister(id string) error {
	f.mu.Lock()
	h, exists := f.handles[id]
	if !exists {
		f.mu.Unlock()
		return ErrUnknownBrowser
	}
	delete(f.handles, id)
	// Read Native BEFORE the handle is gone so the count stays balanced —
	// decrementing on a non-native handle would corrupt the hook gate.
	if h.Native && f.nativeCount > 0 {
		f.nativeCount--
	}
	f.mu.Unlock()

	f.reconcileTakeover()
	return nil
}

// reconcileTakeover installs or uninstalls the mouse-takeover watchdog to match
// the current native-handle count. Idempotent and order-independent: it reads
// the authoritative count rather than trusting a single transition edge, so
// racing Register/Unregister calls converge on the correct state. BiDi-only
// fleets (nativeCount==0) pay zero hook overhead.
func (f *Fleet) reconcileTakeover() {
	f.takeoverMu.Lock()
	defer f.takeoverMu.Unlock()

	f.mu.RLock()
	want := f.nativeCount > 0 && !f.stopped
	f.mu.RUnlock()

	installed := f.watchdog != nil
	switch {
	case want && !installed:
		f.startTakeoverLocked()
	case !want && installed:
		f.stopTakeoverLocked()
	}
}

// startTakeoverLocked spins up the watchdog and installs the low-level mouse
// hook. Caller must hold f.takeoverMu. If the hook fails to install, the
// watchdog is torn down (without a hook it can never fire) and the fleet keeps
// running without takeover — logged, never fatal.
func (f *Fleet) startTakeoverLocked() {
	w := newTakeoverWatchdog(takeoverIdle,
		func() { f.Pause(ReasonUserTakeover) },
		func() { f.Resume(ReasonUserTakeover) })
	w.start()
	uninstall, err := winapi.InstallMouseHook(w.notify)
	if err != nil {
		f.log.Warnf("automationfleet: mouse takeover watchdog disabled: %v", err)
		w.stop()
		return
	}
	f.watchdog = w
	f.hookUninstall = uninstall
	f.log.Infof("automationfleet: mouse takeover watchdog installed")
}

// stopTakeoverLocked uninstalls the hook and stops the watchdog (which clears
// any lingering "user-takeover" pause). Caller must hold f.takeoverMu.
//
// Ordering matters: hookUninstall() first blocks until the message-pump thread
// exits (no more notify() calls can arrive), THEN watchdog.stop() blocks until
// the run loop drains — and stop() resumes before it closes its done channel,
// so once this returns the ReasonUserTakeover pause is guaranteed cleared. A
// concurrent Snapshot()/hasReason() may still observe the pause momentarily
// while teardown is in flight; that is benign (it clears within one tick).
func (f *Fleet) stopTakeoverLocked() {
	if f.hookUninstall != nil {
		f.hookUninstall()
		f.hookUninstall = nil
	}
	if f.watchdog != nil {
		f.watchdog.stop()
		f.watchdog = nil
	}
	f.log.Infof("automationfleet: mouse takeover watchdog removed")
}

// Start spawns dispatcher workers and the hotkey listener (if any hotkey
// enabled). Idempotent — calling twice is a no-op.
func (f *Fleet) Start() {
	f.dispatcher.start()

	bindings := make([]HotkeyBinding, 0, 3)
	if f.cfg.pauseEnabled {
		bindings = append(bindings, HotkeyBinding{
			Hotkey: f.cfg.pauseHotkey,
			OnFire: func() { f.Pause("hotkey") },
		})
	}
	if f.cfg.resumeEnabled {
		bindings = append(bindings, HotkeyBinding{
			Hotkey: f.cfg.resumeHotkey,
			OnFire: func() { f.Resume("hotkey") },
		})
	}
	if f.cfg.stopEnabled {
		bindings = append(bindings, HotkeyBinding{
			Hotkey: f.cfg.stopHotkey,
			OnFire: func() { f.requestStop("hotkey") },
		})
	}
	if len(bindings) == 0 {
		return
	}
	f.hotkeyDone = make(chan struct{})
	go func() {
		defer close(f.hotkeyDone)
		if err := runHotkeyMultiListener(f.ctx, bindings); err != nil {
			f.log.Warnf("automationfleet: hotkey listener: %v", err)
		}
	}()
}

// Pause stops the dispatcher from popping new jobs. In-flight jobs continue
// to completion. Idempotent — calling twice fires OnPause once.
func (f *Fleet) Pause(reason string) {
	if f.dispatcher.pause(reason) {
		f.log.Infof("automationfleet: pause requested (%s)", reason)
		if f.cfg.onPause != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						f.log.Errorf("automationfleet: OnPause panic: %v", r)
					}
				}()
				f.cfg.onPause(reason)
			}()
		}
	}
}

// Resume unblocks the dispatcher to pop pending jobs. Idempotent — fires
// OnResume only when transitioning from paused to running.
func (f *Fleet) Resume(reason string) {
	if f.dispatcher.resume(reason) {
		f.log.Infof("automationfleet: resume requested (%s)", reason)
		if f.cfg.onResume != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						f.log.Errorf("automationfleet: OnResume panic: %v", r)
					}
				}()
				f.cfg.onResume(reason)
			}()
		}
	}
}

// Submit enqueues a job. Returns a buffered channel that receives exactly one
// JobResult — Done, Failed, Cancelled, or Rejected. The channel is closed
// after the result lands.
func (f *Fleet) Submit(j Job) (<-chan JobResult, error) {
	if j.Action == nil {
		return nil, errors.New("automationfleet: Job.Action required")
	}
	if err := j.Action.validate(); err != nil {
		return nil, err
	}
	f.mu.RLock()
	if f.stopped {
		f.mu.RUnlock()
		return nil, ErrFleetStopped
	}
	if _, ok := f.handles[j.BrowserID]; !ok {
		f.mu.RUnlock()
		return nil, ErrUnknownBrowser
	}
	f.mu.RUnlock()

	if j.Timeout <= 0 {
		j.Timeout = f.cfg.defaultTimeout
	}
	return f.dispatcher.enqueue(j), nil
}

// Stop cancels in-flight jobs, drains the queue, and tears down workers.
// Idempotent.
func (f *Fleet) Stop() {
	f.requestStop("stop")
	if f.hotkeyDone != nil {
		<-f.hotkeyDone
	}
}

// Wait blocks until the queue is fully drained (no in-flight, no pending).
// Useful for "submit N then wait" patterns. Returns immediately if stopped.
func (f *Fleet) Wait() {
	f.dispatcher.waitDrained()
}

// requestStop is the single funnel for shutting down the dispatcher.
// Idempotent via stopOnce — safe to call from hotkey, Stop, or AbortAll.
func (f *Fleet) requestStop(reason string) {
	f.stopOnce.Do(func() {
		f.log.Infof("automationfleet: stop requested (%s)", reason)
		f.mu.Lock()
		f.stopped = true
		f.mu.Unlock()
		f.reconcileTakeover() // tear down the takeover hook now that we're stopped
		if f.cfg.onStop != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						f.log.Errorf("automationfleet: OnStop panic: %v", r)
					}
				}()
				f.cfg.onStop(reason)
			}()
		}
		f.dispatcher.abortAll(reason)
		f.cancel()
	})
}

// Snapshot is a point-in-time view of takeover-relevant fleet state, built as a
// new public surface because automationfleet has no generic Stats API. HasNative
// reports whether ≥1 native browser is registered (the takeover hook only runs
// then); AutoPaused reports whether the takeover watchdog currently holds the
// fleet paused via the "user-takeover" reason. Safe to call concurrently (e.g.
// from a 1Hz status tick) — each field is read under its owning lock.
type Snapshot struct {
	HasNative  bool
	AutoPaused bool
}

// Snapshot returns the current takeover-relevant fleet state.
func (f *Fleet) Snapshot() Snapshot {
	f.mu.RLock()
	hasNative := f.nativeCount > 0
	f.mu.RUnlock()
	return Snapshot{
		HasNative:  hasNative,
		AutoPaused: f.dispatcher.hasReason(ReasonUserTakeover),
	}
}

// handle looks up a registered browser by id; returns nil when missing.
func (f *Fleet) handle(id string) *BrowserHandle {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.handles[id]
}
