package platform

import (
	"reflect"
	"testing"
)

func TestIsWindowsDesktopExe(t *testing.T) {
	tests := []struct {
		exe  string
		want bool
	}{
		{`C:\Program Files\WindowsApps\Claude_2.9939.4.0_x64__pzs8sxrjxfjjc\app\Claude.exe`, true},
		{`c:\program files\windowsapps\claude_2.9939.4.0_x64__pzs8sxrjxfjjc\app\CLAUDE.EXE`, true},
		{`C:\Users\u\AppData\Local\AnthropicClaude\claude.exe`, true},
		{`C:\Users\u\AppData\Local\AnthropicClaude\app-1.2.3\claude.exe`, true},
		// Claude Code workers: the path Windows reports inside the package, the real one, and one
		// bundled under the app folder.
		{`C:\Users\u\AppData\Roaming\Claude\claude-code\2.1.284\claude.exe`, false},
		{`C:\Users\u\AppData\Local\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude\claude-code\2.1.284\claude.exe`, false},
		{`C:\Program Files\WindowsApps\Claude_2.9939.4.0_x64__pzs8sxrjxfjjc\app\resources\Claude\claude-code\claude.exe`, false},
		{`C:\Users\u\.local\bin\claude.exe`, false}, // the Claude Code CLI
		{`C:\Program Files\WindowsApps\Claude_2.9939.4.0_x64__pzs8sxrjxfjjc\app\resources\chrome-native-host.exe`, false},
		{`C:\Users\u\AppData\Local\AnthropicClaude\Update.exe`, false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isWindowsDesktopExe(tt.exe); got != tt.want {
			t.Errorf("isWindowsDesktopExe(%q) = %v, want %v", tt.exe, got, tt.want)
		}
	}
}

func TestDesktopFromPS(t *testing.T) {
	out := "    1     0     0 /sbin/launchd\n" +
		"  501     1   501 /Applications/Claude.app/Contents/MacOS/Claude\n" +
		"  502   501   501 /Applications/Claude.app/Contents/Frameworks/Claude Helper (Renderer).app/Contents/MacOS/Claude Helper (Renderer)\n" +
		"  503   501   501 /Users/u/Library/Application Support/Claude/claude-code/2.1.284/claude.app/Contents/MacOS/claude\n" +
		"  504   501   501 /Users/u/Library/Application Support/Claude/claude-code/2.1.284/Claude.app/Contents/MacOS/claude\n" +
		"  600   300   501 /usr/local/bin/claude\n" +
		"  700   1 502 /Users/v/Applications/Claude.app/Contents/MacOS/Claude\n" + // another user's
		"not a process line\n" +
		"  800 1 /Applications/Claude.app/Contents/MacOS/Claude\n" // no uid column
	mine := []Proc{
		{PID: 501, PPID: 1, Exe: "/Applications/Claude.app/Contents/MacOS/Claude", Main: true},
		{PID: 502, PPID: 501, Exe: "/Applications/Claude.app/Contents/Frameworks/Claude Helper (Renderer).app/Contents/MacOS/Claude Helper (Renderer)"},
	}
	if got := desktopFromPS(out, 501); !reflect.DeepEqual(got, mine) {
		t.Errorf("desktopFromPS(uid 501):\n got %+v\nwant %+v", got, mine)
	}
	everyone := append(mine, Proc{PID: 700, PPID: 1, Exe: "/Users/v/Applications/Claude.app/Contents/MacOS/Claude", Main: true})
	if got := desktopFromPS(out, -1); !reflect.DeepEqual(got, everyone) {
		t.Errorf("desktopFromPS(every user):\n got %+v\nwant %+v", got, everyone)
	}
}

func TestIsLinuxDesktop(t *testing.T) {
	tests := []struct {
		exe  string
		args []string
		want bool
	}{
		{"/usr/lib/claude-desktop/claude-desktop", []string{"/usr/bin/claude-desktop"}, true},
		{"/usr/lib/claude-desktop-unofficial/claude-desktop", []string{"claude-desktop", "--type=renderer"}, true},
		{"", []string{"/usr/bin/claude-desktop"}, true}, // exe unreadable, argv0 still tells
		{"/usr/lib/electron37/electron", []string{"electron", "/usr/lib/claude-desktop-unofficial/app.asar"}, true},
		{"/usr/bin/grep", []string{"grep", "claude-desktop"}, false},
		{"/home/u/.config/Claude/claude-code/2.1.284/claude", []string{"claude"}, false},
		{"/usr/lib/electron37/electron", []string{"electron", "/opt/other/app.asar"}, false},
		{"", nil, false},
	}
	for _, tt := range tests {
		if got := isLinuxDesktop(tt.exe, tt.args); got != tt.want {
			t.Errorf("isLinuxDesktop(%q, %q) = %v, want %v", tt.exe, tt.args, got, tt.want)
		}
	}
}

func TestStatPPID(t *testing.T) {
	tests := map[string]int{
		"1234 (claude-desktop) S 1000 1234 1234 0 -1": 1000,
		"77 (a b) (c) R 42 1 1":                       42,
		"":                                            0,
	}
	for stat, want := range tests {
		if got := statPPID(stat); got != want {
			t.Errorf("statPPID(%q) = %d, want %d", stat, got, want)
		}
	}
}

func TestStatusUID(t *testing.T) {
	tests := map[string]int{
		"Name:\tchrome-sandbox\nPPid:\t1\nUid:\t1000\t0\t0\t0\nGid:\t1000\n": 1000, // setuid: the real uid counts
		"Name:\tx\n": -1, // the process exited between reads
		"":           -1,
	}
	for status, want := range tests {
		if got := statusUID(status); got != want {
			t.Errorf("statusUID(%q) = %d, want %d", status, got, want)
		}
	}
}

func TestMarkMain(t *testing.T) {
	ps := []Proc{{PID: 1, PPID: 100}, {PID: 2, PPID: 1}, {PID: 3, PPID: 2}, {PID: 4, PPID: 999}}
	markMain(ps)
	for _, p := range ps {
		if want := p.PID == 1 || p.PID == 4; p.Main != want {
			t.Errorf("pid %d: Main = %v, want %v", p.PID, p.Main, want)
		}
	}
}
