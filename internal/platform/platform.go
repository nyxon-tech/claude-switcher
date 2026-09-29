// Package platform knows where Claude Desktop keeps its data on each OS, which processes belong
// to it, and how to quit and start it.
package platform

import (
	"errors"
	"os"
	"path/filepath"
)

// Kind is how Claude Desktop was installed.
type Kind string

const (
	MSIX     Kind = "msix"     // Windows, current installer (package Claude_pzs8sxrjxfjjc)
	Squirrel Kind = "squirrel" // Windows, older .exe installer (%LOCALAPPDATA%\AnthropicClaude)
	MacApp   Kind = "macos"    // /Applications/Claude.app
	Linux    Kind = "linux"    // official .deb or community builds
	Custom   Kind = "custom"   // a folder given with --data-dir (tests, portable setups)
)

// Install is one Claude Desktop installation found on this machine.
type Install struct {
	Kind    Kind
	DataDir string // Desktop's data folder: holds config.json and claude-code-sessions
	LogDir  string // Desktop's log folder, "" when unknown
	AppPath string // executable or .app bundle used to start it, "" when unknown
	AppID   string // Windows AUMID (MSIX) or macOS bundle id, "" when not applicable
}

// Desktop controls a running Claude Desktop. ops only writes Claude's data after Running
// reports false. Tests use a fake.
type Desktop interface {
	// Running reports whether any Desktop process (the app or its helpers) is alive.
	// A Claude Code CLI running in a terminal is not Desktop.
	Running() (bool, error)
	// Quit asks Desktop to quit the way a user would. It returns without waiting.
	Quit() error
	// ForceQuit ends every Desktop process. Only after the user agreed to it.
	ForceQuit() error
	// Launch starts Desktop again.
	Launch() error
	// StopHelpers ends helper processes that keep Desktop's files open once Desktop is closed
	// (the Claude in Chrome native host). Best effort.
	StopHelpers()
}

// ErrManualQuit is returned by Quit when the OS offers no reliable way to quit Desktop from
// outside (Windows: it keeps running in the tray). The user has to quit it from the tray.
var ErrManualQuit = errors.New("quit claude desktop from its tray icon")

// ErrNotFound is returned by Detect when no Claude Desktop installation exists.
var ErrNotFound = errors.New("claude desktop not found")

// LoginItems are the files and folders in DataDir that make Desktop signed in as one account.
// They are copied as opaque files, never parsed. Items that do not exist are skipped.
var LoginItems = []string{
	"config.json", "Preferences",
	"DIPS", "DIPS-wal",
	"SharedStorage", "SharedStorage-wal",
	"InterestGroups", "InterestGroups-wal",
	"Cookies", "Cookies-journal", // macOS and Linux keep cookies at the top level
	"ant-did", "buddy-tokens.json", "plan-usage-history.json", "cowork-enabled-cli-ops.json",
	"Local Storage", "Session Storage", "Network", "IndexedDB", "WebStorage", "Shared Dictionary",
}

// Home is the user's home folder.
func Home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// ClaudeHome is Claude Code's folder: $CLAUDE_CONFIG_DIR or ~/.claude.
func ClaudeHome() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(Home(), ".claude")
}

// ProjectsDir holds every Claude Code transcript.
func ProjectsDir() string { return filepath.Join(ClaudeHome(), "projects") }

// VaultDir holds our own state. It sits in the home folder on purpose: on Windows, MSIX apps
// see a redirected %LOCALAPPDATA%.
func VaultDir() string {
	if d := os.Getenv("CLAUDE_SWITCHER_HOME"); d != "" {
		return d
	}
	return filepath.Join(Home(), ".claude-instances")
}

// Detect returns every Claude Desktop installation on this machine, the one in use first
// (implemented per OS in detect_<os>.go).
func Detect() ([]Install, error) { return detect() }

// NewDesktop returns the controller for an installation (implemented per OS).
func NewDesktop(in Install) Desktop { return newDesktop(in) }
