package winapi

// LLMHFInjected mirrors the Win32 MSLLHOOKSTRUCT.flags bit that Windows sets on
// events produced by SendInput (i.e. our own automation). Real hardware moves
// never carry it. Kept build-tag-free so the classifier below is unit-testable
// on every platform, decoupled from the OS hook.
const LLMHFInjected = 0x00000001

// IsRealMouseMove reports whether a low-level mouse event's flags field denotes
// genuine hardware input rather than a SendInput-injected (automation) event.
func IsRealMouseMove(flags uint32) bool { return flags&LLMHFInjected == 0 }
