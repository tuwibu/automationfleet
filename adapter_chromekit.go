package automationfleet

import (
	"context"
	"fmt"
	"time"

	"github.com/tuwibu/chromekit"
)

// WrapChrome adapts a chromekit.Browser into a fleet Driver.
func WrapChrome(b *chromekit.Browser) Driver { return chromeDriver{b} }

type chromeDriver struct{ b *chromekit.Browser }

func (d chromeDriver) Current() Page {
	p := d.b.Current()
	if p == nil {
		return nil // never wrap nil — chromePage{nil} would be a non-nil Page interface
	}
	return chromePage{p}
}

func (d chromeDriver) Focus(ctx context.Context) error { return d.b.Focus(ctx) }

func (d chromeDriver) ContentOffset() (int, int, error) { return d.b.ContentOffset() }

func (d chromeDriver) InputBackend() Backend {
	if d.b.InputBackend() == chromekit.BackendNative {
		return BackendNative
	}
	return BackendRemote
}

type chromePage struct{ p *chromekit.Page }

func (p chromePage) Mouse() Mouse       { return chromeMouse{p.p.Mouse()} }
func (p chromePage) Keyboard() Keyboard { return chromeKeyboard{p.p.Keyboard()} }

func (p chromePage) Evaluate(expr string, out any) error { return p.p.Evaluate(expr, out) }

func (p chromePage) Navigate(url string, timeout time.Duration) error {
	return p.p.Navigate(url, timeout)
}

func (p chromePage) WaitForSelector(selector string, timeout time.Duration) error {
	return p.p.WaitForSelector(selector, timeout)
}

// ElementCenter mirrors the original native scrollAndQueryCenter flow: scroll
// the element to viewport center, let layout settle, re-query the box (it can
// move after scroll), and return its center in CSS pixels. chromekit's
// BoundingBox sets X/Y == Left/Top, so X/Y is the top-left origin.
func (p chromePage) ElementCenter(selector string) (float64, float64, error) {
	scrollExpr := fmt.Sprintf(
		`(()=>{const el=document.querySelector(%q); if(!el) return false; el.scrollIntoView({block:'center', inline:'center'}); return true})()`,
		selector,
	)
	var ok bool
	if err := p.p.Evaluate(scrollExpr, &ok); err != nil {
		return 0, 0, fmt.Errorf("scrollIntoView: %w", err)
	}
	if !ok {
		return 0, 0, fmt.Errorf("element %q not found", selector)
	}
	time.Sleep(60 * time.Millisecond)

	node, err := p.p.QuerySelector(selector, 2*time.Second)
	if err != nil {
		return 0, 0, err
	}
	box, err := p.p.BoundingBox(node)
	if err != nil {
		return 0, 0, err
	}
	if box.Width <= 0 || box.Height <= 0 {
		return 0, 0, fmt.Errorf("zero-size element %q", selector)
	}
	return box.X + box.Width/2, box.Y + box.Height/2, nil
}

type chromeMouse struct{ m *chromekit.MouseAPI }

func (m chromeMouse) MoveTo(x, y float64) error       { return m.m.MoveTo(x, y) }
func (m chromeMouse) ClickAt(x, y float64) error      { return m.m.ClickAt(x, y) }
func (m chromeMouse) Click(selector string) error     { return m.m.Click(selector) }
func (m chromeMouse) FocusElement(selector string) error { return m.m.FocusElement(selector) }

type chromeKeyboard struct{ k *chromekit.KeyboardAPI }

func (k chromeKeyboard) ClearInput(selector ...string) error { return k.k.ClearInput(selector...) }
func (k chromeKeyboard) TypeHuman(text string) error         { return k.k.TypeHuman(text) }
