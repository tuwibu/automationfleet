package automationfleet

import (
	"fmt"
	"strconv"
	"strings"
)

// Modifier is a bitmask of modifier keys for RegisterHotKey.
// Values match Windows MOD_* constants so the Windows listener can pass
// directly without translation.
type Modifier uint32

const (
	ModAlt   Modifier = 0x0001
	ModCtrl  Modifier = 0x0002
	ModShift Modifier = 0x0004
	ModWin   Modifier = 0x0008
)

// Key is a Windows virtual-key code. Only the keys we expose are listed;
// callers can pass raw VK_ values for anything else.
type Key uint32

const (
	KeyA Key = 0x41
	KeyB Key = 0x42
	KeyC Key = 0x43
	KeyD Key = 0x44
	KeyE Key = 0x45
	KeyF Key = 0x46
	KeyG Key = 0x47
	KeyH Key = 0x48
	KeyI Key = 0x49
	KeyJ Key = 0x4A
	KeyK Key = 0x4B
	KeyL Key = 0x4C
	KeyM Key = 0x4D
	KeyN Key = 0x4E
	KeyO Key = 0x4F
	KeyP Key = 0x50
	KeyQ Key = 0x51
	KeyR Key = 0x52
	KeyS Key = 0x53
	KeyT Key = 0x54
	KeyU Key = 0x55
	KeyV Key = 0x56
	KeyW Key = 0x57
	KeyX Key = 0x58
	KeyY Key = 0x59
	KeyZ Key = 0x5A

	KeyF1  Key = 0x70
	KeyF2  Key = 0x71
	KeyF3  Key = 0x72
	KeyF4  Key = 0x73
	KeyF5  Key = 0x74
	KeyF6  Key = 0x75
	KeyF7  Key = 0x76
	KeyF8  Key = 0x77
	KeyF9  Key = 0x78
	KeyF10 Key = 0x79
	KeyF11 Key = 0x7A
	KeyF12 Key = 0x7B
)

// Hotkey is the registered combo. Mods may OR multiple Modifier values.
type Hotkey struct {
	Mods Modifier
	Key  Key
}

// DefaultStopHotkey is Ctrl+Alt+Shift+S — chosen for low collision risk with
// common system shortcuts.
var DefaultStopHotkey = Hotkey{Mods: ModCtrl | ModAlt | ModShift, Key: KeyS}

// String renders a Hotkey as a human-readable combo (e.g. "Ctrl+Alt+Shift+S").
func (h Hotkey) String() string {
	parts := []string{}
	if h.Mods&ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if h.Mods&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if h.Mods&ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if h.Mods&ModWin != 0 {
		parts = append(parts, "Win")
	}
	switch {
	case h.Key >= KeyA && h.Key <= KeyZ:
		parts = append(parts, string(rune('A'+(h.Key-KeyA))))
	case h.Key >= KeyF1 && h.Key <= KeyF12:
		parts = append(parts, fmt.Sprintf("F%d", int(h.Key-KeyF1+1)))
	default:
		parts = append(parts, fmt.Sprintf("VK_0x%X", uint32(h.Key)))
	}
	return strings.Join(parts, "+")
}

// ParseHotkey accepts strings like "Ctrl+Alt+Shift+S" or "Ctrl+Q".
// Letter keys are case-insensitive. Unknown tokens return an error.
func ParseHotkey(s string) (Hotkey, error) {
	out := Hotkey{}
	tokens := strings.Split(s, "+")
	if len(tokens) == 0 {
		return out, fmt.Errorf("hotkey: empty string")
	}
	for i, raw := range tokens {
		tok := strings.TrimSpace(raw)
		if tok == "" {
			return out, fmt.Errorf("hotkey: empty token at position %d", i)
		}
		switch strings.ToLower(tok) {
		case "ctrl", "control":
			out.Mods |= ModCtrl
		case "alt":
			out.Mods |= ModAlt
		case "shift":
			out.Mods |= ModShift
		case "win", "super", "meta":
			out.Mods |= ModWin
		default:
			low := strings.ToLower(tok)
			if len(tok) == 1 {
				ch := strings.ToUpper(tok)[0]
				if ch >= 'A' && ch <= 'Z' {
					out.Key = Key(ch)
					continue
				}
			}
			if len(low) >= 2 && low[0] == 'f' {
				if n, err := strconv.Atoi(low[1:]); err == nil && n >= 1 && n <= 12 {
					out.Key = KeyF1 + Key(n-1)
					continue
				}
			}
			return out, fmt.Errorf("hotkey: unknown token %q", tok)
		}
	}
	if out.Key == 0 {
		return out, fmt.Errorf("hotkey: missing primary key in %q", s)
	}
	if out.Mods == 0 {
		return out, fmt.Errorf("hotkey: at least one modifier required in %q", s)
	}
	return out, nil
}
