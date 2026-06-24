//go:build windows

package automationfleet

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/tuwibu/automationfleet/internal/winapi"
)

// executeCritical runs the atomic native sequence for a single job:
//
//	focus → scrollIntoView → re-query bbox → MoveTo → cursor drift guard
//	→ Click  [→ IME guard → Type → restore]
//
// The whole sequence holds the OS focus and runs on the dispatcher's single
// native worker — chromekit's package-level mutex is a defense-in-depth net,
// the contract here is "this function is the sole native input source".
func executeCritical(ctx context.Context, f *Fleet, h *BrowserHandle, a Action) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := h.Driver.Focus(ctx); err != nil {
		return fmt.Errorf("focus: %w", err)
	}

	page := h.Driver.Current()
	if page == nil {
		return fmt.Errorf("no active page on browser %s", h.ID)
	}

	switch act := a.(type) {
	case NavigateAction:
		// Browser already focused above; Page.Navigate handles the
		// omnibox keystroke sequence. Native input mutex inside chromekit
		// is defense-in-depth; the dispatcher's single native worker is
		// the real guarantee.
		timeout := act.Timeout
		if timeout <= 0 {
			timeout = f.cfg.defaultTimeout
		}
		return page.Navigate(act.URL, timeout)

	case ClickAction:
		if err := ctx.Err(); err != nil {
			return err
		}
		x, y, err := page.ElementCenter(act.Selector)
		if err != nil {
			return err
		}
		return clickAt(ctx, f, h, page, x, y)

	case TypeAction:
		if err := ctx.Err(); err != nil {
			return err
		}
		x, y, err := page.ElementCenter(act.Selector)
		if err != nil {
			return err
		}
		if err := clickAt(ctx, f, h, page, x, y); err != nil {
			return err
		}
		// brief settle so focus lands inside the input element
		time.Sleep(80 * time.Millisecond)

		prevLayout, layoutErr := winapi.ForceENUSLayout()
		if layoutErr != nil {
			f.log.Warnf("automationfleet: layout switch failed: %v (typing anyway)", layoutErr)
		}
		defer winapi.RestoreLayout(prevLayout)

		if act.ClearFirst {
			// Already focused after clickAt — ClearInput w/o selector
			// does Ctrl+A + Delete on the focused element.
			if err := page.Keyboard().ClearInput(); err != nil {
				return fmt.Errorf("clearInput: %w", err)
			}
		}
		// TypeHuman: 80-220ms/char + 5% typo (backspace + retype). Mechanical
		// Keyboard.Type 50-150ms không có typo → anti-bot detect.
		return page.Keyboard().TypeHuman(act.Text)

	default:
		return fmt.Errorf("executeCritical: unsupported action %s", a.kind())
	}
}

// clickAt drives Page.MoveTo + ClickAt while guarding against cursor drift
// (a human grabbing the mouse mid-flight). On drift, returns errCursorDrift
// so the caller's retry path kicks in.
func clickAt(ctx context.Context, f *Fleet, h *BrowserHandle, page Page, cssX, cssY float64) error {
	if err := page.Mouse().MoveTo(cssX, cssY); err != nil {
		return fmt.Errorf("moveTo: %w", err)
	}

	expectedX := h.X + int(math.Round(cssX*h.Scale))
	expectedY := h.Y + int(math.Round(cssY*h.Scale))
	// chromekit's native backend adds chrome-chrome offset (title bar + tabs
	// + omnibox) so CSS (0,0) lands on content origin, not window top-left.
	// Mirror it here so the drift check compares apples to apples.
	if ox, oy, err := h.Driver.ContentOffset(); err == nil {
		expectedX += ox
		expectedY += oy
	}
	gotX, gotY, err := winapi.GetCursorPos()
	if err == nil {
		if abs(gotX-expectedX) > f.cfg.driftThreshold || abs(gotY-expectedY) > f.cfg.driftThreshold {
			return fmt.Errorf("%w: expected (%d,%d) got (%d,%d)", errCursorDrift,
				expectedX, expectedY, gotX, gotY)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := page.Mouse().ClickAt(cssX, cssY); err != nil {
		return fmt.Errorf("click: %w", err)
	}
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
