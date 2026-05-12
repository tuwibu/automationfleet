//go:build !windows

package chromefleet

import (
	"context"
	"fmt"
	"sync"
)

var warnCDPFallbackOnce sync.Once

// executeCritical on non-Windows degrades to CDP-only input. The result is
// detectable by anti-bot scripts (no real focus / cursor / OS keystrokes), but
// it keeps the API working for development on macOS / Linux. Production use
// requires Windows.
func executeCritical(_ context.Context, f *Fleet, h *BrowserHandle, a Action) error {
	warnCDPFallbackOnce.Do(func() {
		f.log.Warnf("chromefleet: native critical section unavailable — falling back to CDP input (no anti-bot guarantees)")
	})

	page := h.Browser.Current()
	if page == nil {
		return fmt.Errorf("no active page on browser %s", h.ID)
	}

	switch act := a.(type) {
	case NavigateAction:
		timeout := act.Timeout
		if timeout <= 0 {
			timeout = f.cfg.defaultTimeout
		}
		return page.Navigate(act.URL, timeout)
	case ClickAction:
		return page.Mouse().Click(act.Selector)
	case TypeAction:
		if err := page.Mouse().FocusElement(act.Selector); err != nil {
			return err
		}
		if act.ClearFirst {
			if err := page.Keyboard().ClearInput(); err != nil {
				return err
			}
		}
		return page.Keyboard().TypeHuman(act.Text)
	default:
		return fmt.Errorf("executeCritical: unsupported action %s", a.kind())
	}
}
