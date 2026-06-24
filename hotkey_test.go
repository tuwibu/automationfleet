package automationfleet

import "testing"

func TestParseHotkey_F10WithCtrl(t *testing.T) {
	got, err := ParseHotkey("Ctrl+F10")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Mods != ModCtrl {
		t.Errorf("Mods = %v, want ModCtrl", got.Mods)
	}
	if got.Key != KeyF10 {
		t.Errorf("Key = %v, want KeyF10", got.Key)
	}
}

func TestParseHotkey_F11WithCtrlLowercase(t *testing.T) {
	got, err := ParseHotkey("ctrl+f11")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Mods != ModCtrl {
		t.Errorf("Mods = %v, want ModCtrl", got.Mods)
	}
	if got.Key != KeyF11 {
		t.Errorf("Key = %v, want KeyF11", got.Key)
	}
}

func TestHotkey_StringRendersFKey(t *testing.T) {
	h := Hotkey{Mods: ModCtrl, Key: KeyF11}
	if got := h.String(); got != "Ctrl+F11" {
		t.Errorf("String() = %q, want %q", got, "Ctrl+F11")
	}
}

func TestHotkey_StringRendersF1(t *testing.T) {
	h := Hotkey{Mods: ModCtrl | ModShift, Key: KeyF1}
	if got := h.String(); got != "Ctrl+Shift+F1" {
		t.Errorf("String() = %q, want %q", got, "Ctrl+Shift+F1")
	}
}
