package winapi

import "testing"

// TestIsRealMouseMove verifies the injected-vs-real classifier: SendInput
// (automation) events set LLMHF_INJECTED and must be ignored; real hardware
// events (flag clear) must be reported as real. This is the crux of not
// auto-pausing on our own automation moves.
func TestIsRealMouseMove(t *testing.T) {
	cases := []struct {
		name  string
		flags uint32
		want  bool
	}{
		{"real hardware move (no flags)", 0x00000000, true},
		{"injected via SendInput", LLMHFInjected, false},
		{"injected + lower-IL bit", LLMHFInjected | 0x00000002, false},
		{"lower-IL injected only", 0x00000002, true}, // bit1 alone is not LLMHF_INJECTED
	}
	for _, c := range cases {
		if got := IsRealMouseMove(c.flags); got != c.want {
			t.Errorf("%s: IsRealMouseMove(%#x) = %v, want %v", c.name, c.flags, got, c.want)
		}
	}
}
