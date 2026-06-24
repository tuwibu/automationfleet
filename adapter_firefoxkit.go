package automationfleet

import (
	"context"
	"fmt"
	"time"

	"github.com/tuwibu/firefoxkit"
)

// WrapFirefox adapts a firefoxkit.Browser into a fleet Driver.
//
// NOTE on native input (RT-F7): firefoxkit builds its native input Window from
// WithNativeWindow's X/Y/Scale but leaves ContentOffsetX/Y at zero
// (firefoxkit/page_input_backend.go), so CSS (0,0) maps to the window top-left
// (tabs/omnibox), not the content origin. Native firefox clicks therefore land
// at the wrong position regardless of the fleet's drift guard. Until firefoxkit
// wires MeasureContentOffset into its input Window, register firefox browsers
// with native=false (BiDi/Remote path), which is fully supported.
func WrapFirefox(b *firefoxkit.Browser) Driver { return firefoxDriver{b} }

type firefoxDriver struct{ b *firefoxkit.Browser }

func (d firefoxDriver) Current() Page {
	p := d.b.Current()
	if p == nil {
		return nil
	}
	return firefoxPage{p}
}

func (d firefoxDriver) Focus(ctx context.Context) error { return d.b.Focus(ctx) }

func (d firefoxDriver) ContentOffset() (int, int, error) { return d.b.ContentOffset() }

func (d firefoxDriver) InputBackend() Backend {
	if d.b.InputBackend() == firefoxkit.BackendNative {
		return BackendNative
	}
	return BackendRemote // BackendBiDi → Remote
}

type firefoxPage struct{ p *firefoxkit.Page }

func (p firefoxPage) Mouse() Mouse       { return firefoxMouse{p.p.Mouse()} }
func (p firefoxPage) Keyboard() Keyboard { return firefoxKeyboard{p.p.Keyboard()} }

func (p firefoxPage) Evaluate(expr string, out any) error { return p.p.Evaluate(expr, out) }

func (p firefoxPage) Navigate(url string, timeout time.Duration) error {
	return p.p.Navigate(url, timeout)
}

func (p firefoxPage) WaitForSelector(selector string, timeout time.Duration) error {
	return p.p.WaitForSelector(selector, timeout)
}

// ElementCenter uses firefoxkit's selector-based ScrollIntoView + BoundingBox.
// firefox BoundingBox X/Y is the top-left origin (CSS px, viewport-relative).
func (p firefoxPage) ElementCenter(selector string) (float64, float64, error) {
	if err := p.p.ScrollIntoView(selector); err != nil {
		return 0, 0, err
	}
	bb, err := p.p.BoundingBox(selector)
	if err != nil {
		return 0, 0, err
	}
	if bb.Width <= 0 || bb.Height <= 0 {
		return 0, 0, fmt.Errorf("zero-size element %q", selector)
	}
	return bb.X + bb.Width/2, bb.Y + bb.Height/2, nil
}

type firefoxMouse struct{ m *firefoxkit.MouseAPI }

func (m firefoxMouse) MoveTo(x, y float64) error         { return m.m.MoveTo(x, y) }
func (m firefoxMouse) ClickAt(x, y float64) error        { return m.m.ClickAt(x, y) }
func (m firefoxMouse) Click(selector string) error       { return m.m.Click(selector) }
func (m firefoxMouse) FocusElement(selector string) error { return m.m.FocusElement(selector) }

type firefoxKeyboard struct{ k *firefoxkit.KeyboardAPI }

func (k firefoxKeyboard) ClearInput(selector ...string) error { return k.k.ClearInput(selector...) }
func (k firefoxKeyboard) TypeHuman(text string) error         { return k.k.TypeHuman(text) }
