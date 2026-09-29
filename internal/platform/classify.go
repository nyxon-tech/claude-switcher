package platform

import (
	"path"
	"slices"
	"strconv"
	"strings"
)

// Pure parsers and classifiers for every OS. They carry no build tag so their tests run on
// every CI runner.

// isWindowsDesktopExe reports whether a Windows image path is Claude Desktop (MSIX or
// Squirrel). Claude Code workers and the Claude Code CLI are also named claude.exe.
func isWindowsDesktopExe(exe string) bool {
	p := strings.ToLower(exe)
	if p[strings.LastIndexAny(p, `\/`)+1:] != "claude.exe" || strings.Contains(p, `\claude\claude-code\`) {
		return false
	}
	return strings.Contains(p, `\windowsapps\claude_`) || strings.Contains(p, `\anthropicclaude\`)
}

// desktopFromPS picks the Desktop processes run by uid (every user's when uid < 0) out of
// `ps -axo pid=,ppid=,ruid=,comm=` output. On macOS comm is the full executable path, which
// may contain spaces.
func desktopFromPS(out string, uid int) []Proc {
	var ps []Proc
	for _, line := range strings.Split(out, "\n") {
		pid, rest := cutField(line)
		ppid, rest := cutField(rest)
		owner, rest := cutField(rest)
		exe := strings.TrimSpace(rest)
		p, err1 := strconv.Atoi(pid)
		pp, err2 := strconv.Atoi(ppid)
		u, err3 := strconv.Atoi(owner)
		if err1 != nil || err2 != nil || err3 != nil || uid >= 0 && u != uid || !isMacDesktopExe(exe) {
			continue
		}
		main := strings.HasSuffix(exe, "/Claude.app/Contents/MacOS/Claude")
		ps = append(ps, Proc{PID: p, PPID: pp, Exe: exe, Main: main})
	}
	return ps
}

func cutField(s string) (field, rest string) {
	field, rest, _ = strings.Cut(strings.TrimLeft(s, " \t"), " ")
	return field, rest
}

// isMacDesktopExe reports whether a macOS executable belongs to Claude.app, leaving out the
// Claude Code workers Desktop keeps in its data folder.
func isMacDesktopExe(exe string) bool {
	return strings.Contains(exe, "/Claude.app/Contents/") &&
		!strings.Contains(exe, "/Application Support/Claude/claude-code")
}

// isLinuxDesktop reports whether a Linux process is Claude Desktop: the official
// claude-desktop, the community claude-desktop-unofficial, or a system Electron running
// either app.
func isLinuxDesktop(exe string, args []string) bool {
	if strings.Contains(exe, "claude-desktop") || len(args) > 0 && strings.Contains(args[0], "claude-desktop") {
		return true
	}
	return strings.HasPrefix(path.Base(exe), "electron") &&
		slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "claude-desktop") })
}

// statPPID reads the parent PID from /proc/<pid>/stat. The command name in parentheses may
// itself contain spaces and parentheses, so fields are counted from the last ')'.
func statPPID(stat string) int {
	f := strings.Fields(stat[strings.LastIndexByte(stat, ')')+1:])
	if len(f) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(f[1])
	return ppid
}

// statusUID reads the real user id from /proc/<pid>/status, or -1 when it is missing.
func statusUID(status string) int {
	_, rest, _ := strings.Cut(status, "\nUid:")
	f := strings.Fields(rest) // real, effective, saved, filesystem
	if len(f) == 0 {
		return -1
	}
	uid, err := strconv.Atoi(f[0])
	if err != nil {
		return -1
	}
	return uid
}
