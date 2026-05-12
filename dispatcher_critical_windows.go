//go:build windows

package chromefleet

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/tuwibu/chromekit"
	"github.com/tuwibu/chromefleet/internal/winapi"
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

	if err := h.Browser.Focus(ctx); err != nil {
		return fmt.Errorf("focus: %w", err)
	}

	page := h.Browser.Current()
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
		x, y, err := scrollAndQueryCenter(ctx, page, act.Selector)
		if err != nil {
			return err
		}
		return clickAt(ctx, f, h, page, x, y)

	case TypeAction:
		x, y, err := scrollAndQueryCenter(ctx, page, act.Selector)
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
			f.log.Warnf("chromefleet: layout switch failed: %v (typing anyway)", layoutErr)
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

// scrollAndQueryCenter scrolls the element into view, waits for layout to
// settle, then re-queries the bounding box and returns its center in CSS
// pixels (viewport-relative). Re-query is essential — the box can move after
// scrollIntoView if anything is animating or lazy-loading.
func scrollAndQueryCenter(ctx context.Context, page *chromekit.Page, selector string) (float64, float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	scrollExpr := fmt.Sprintf(
		`(()=>{const el=document.querySelector(%q); if(!el) return false; el.scrollIntoView({block:'center', inline:'center'}); return true})()`,
		selector,
	)
	var ok bool
	if err := page.Evaluate(scrollExpr, &ok); err != nil {
		return 0, 0, fmt.Errorf("scrollIntoView: %w", err)
	}
	if !ok {
		return 0, 0, fmt.Errorf("element %q not found", selector)
	}
	time.Sleep(60 * time.Millisecond)

	node, err := page.QuerySelector(selector, 2*time.Second)
	if err != nil {
		return 0, 0, err
	}
	box, err := page.BoundingBox(node)
	if err != nil {
		return 0, 0, err
	}
	if box.Width <= 0 || box.Height <= 0 {
		return 0, 0, fmt.Errorf("zero-size element %q", selector)
	}
	return box.Left + box.Width/2, box.Top + box.Height/2, nil
}

// clickAt drives Page.MoveTo + ClickAt while guarding against cursor drift
// (a human grabbing the mouse mid-flight). On drift, returns errCursorDrift
// so the caller's retry path kicks in.
func clickAt(ctx context.Context, f *Fleet, h *BrowserHandle, page *chromekit.Page, cssX, cssY float64) error {
	if err := page.Mouse().MoveTo(cssX, cssY); err != nil {
		return fmt.Errorf("moveTo: %w", err)
	}

	expectedX := h.X + int(math.Round(cssX*h.Scale))
	expectedY := h.Y + int(math.Round(cssY*h.Scale))
	// chromekit's native backend adds chrome-chrome offset (title bar + tabs
	// + omnibox) so CSS (0,0) lands on content origin, not window top-left.
	// Mirror it here so the drift check compares apples to apples.
	if ox, oy, err := h.Browser.ContentOffset(); err == nil {
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
