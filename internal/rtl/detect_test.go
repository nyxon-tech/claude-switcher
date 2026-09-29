package rtl

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		goos string
		want Mode
	}{
		{"conhost", nil, "windows", App},
		{"windows terminal", map[string]string{"WT_SESSION": "1f2e"}, "windows", App},
		{"vs code", map[string]string{"TERM_PROGRAM": "vscode"}, "darwin", App},
		{"terminal.app", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, "darwin", Terminal},
		{"iterm2 3.7", map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.7.3"}, "darwin", Terminal},
		{"iterm2 4", map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "4.0.0"}, "darwin", Terminal},
		{"iterm2 3.6", map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.6.0beta3"}, "darwin", App},
		{"iterm2 unknown version", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "darwin", App},
		{"konsole", map[string]string{"KONSOLE_VERSION": "240202"}, "linux", Terminal},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, "linux", App},
		{"vte", map[string]string{"VTE_VERSION": "7600"}, "linux", App},
		{"mintty", map[string]string{"TERM_PROGRAM": "mintty"}, "windows", App},
		{"alacritty", map[string]string{"ALACRITTY_WINDOW_ID": "1"}, "linux", App},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, "darwin", App},
		{"kitty", map[string]string{"TERM": "xterm-kitty", "KITTY_WINDOW_ID": "1"}, "linux", App},
		{"tmux", map[string]string{"TERM_PROGRAM": "tmux"}, "linux", App},
		{"unknown", nil, "linux", App},
		{"override terminal", map[string]string{"CLAUDE_SWITCHER_RTL": "terminal", "WT_SESSION": "x"}, "windows", Terminal},
		{"override off", map[string]string{"CLAUDE_SWITCHER_RTL": "off"}, "windows", Off},
		{"override app", map[string]string{"CLAUDE_SWITCHER_RTL": "App", "TERM_PROGRAM": "Apple_Terminal"}, "darwin", App},
		{"override auto detects", map[string]string{"CLAUDE_SWITCHER_RTL": "auto", "TERM_PROGRAM": "Apple_Terminal"}, "darwin", Terminal},
		{"bad override ignored", map[string]string{"CLAUDE_SWITCHER_RTL": "yes", "TERM_PROGRAM": "Apple_Terminal"}, "darwin", Terminal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			if got := Detect(getenv, tt.goos); got != tt.want {
				t.Errorf("Detect = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseMode(t *testing.T) {
	for _, m := range []Mode{Auto, App, Terminal, Off} {
		if got, ok := ParseMode(m.String()); !ok || got != m {
			t.Errorf("ParseMode(%q) = %v, %v", m.String(), got, ok)
		}
	}
	if got, ok := ParseMode(" Terminal "); !ok || got != Terminal {
		t.Errorf("ParseMode is not lenient: %v, %v", got, ok)
	}
	if _, ok := ParseMode("rtl"); ok {
		t.Error(`ParseMode("rtl") succeeded`)
	}
	if got := Mode(9).String(); got != "Mode(9)" {
		t.Errorf("Mode(9).String() = %q", got)
	}
}
