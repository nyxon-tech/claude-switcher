package rtl

import (
	"fmt"
	"strings"
)

var modeNames = [...]string{Auto: "auto", App: "app", Terminal: "terminal", Off: "off"}

// Detect picks App or Terminal for the terminal this process runs in. getenv is os.Getenv in
// production; goos is runtime.GOOS. It never returns Auto.
//
// CLAUDE_SWITCHER_RTL (app, terminal or off) overrides the detection. Only Terminal.app,
// iTerm2 3.7+ and Konsole join and reorder by default; every other terminal, on every OS,
// gets App. VTE, WezTerm and mintty reorder too unless switched to explicit mode (CSI 8 l).
func Detect(getenv func(string) string, goos string) Mode {
	if m, ok := ParseMode(getenv("CLAUDE_SWITCHER_RTL")); ok && m != Auto {
		return m
	}
	if getenv("WT_SESSION") != "" {
		return App
	}
	switch getenv("TERM_PROGRAM") {
	case "Apple_Terminal":
		return Terminal
	case "iTerm.app":
		var major, minor int
		if _, err := fmt.Sscanf(getenv("TERM_PROGRAM_VERSION"), "%d.%d", &major, &minor); err == nil &&
			(major > 3 || major == 3 && minor >= 7) {
			return Terminal
		}
		return App
	}
	if getenv("KONSOLE_VERSION") != "" {
		return Terminal
	}
	return App
}

// ParseMode reads "auto", "app", "terminal" or "off".
func ParseMode(s string) (Mode, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for m, name := range modeNames {
		if name == s {
			return Mode(m), true
		}
	}
	return Auto, false
}

// String is the name ParseMode reads.
func (m Mode) String() string {
	if m < 0 || int(m) >= len(modeNames) {
		return fmt.Sprintf("Mode(%d)", int(m))
	}
	return modeNames[m]
}
