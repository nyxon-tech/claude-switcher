package ui

import (
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

// Options configure the full-screen app. The CLI fills them from flags and saved settings.
type Options struct {
	Version  string   // "3.0.0", shown in the footer
	Mode     rtl.Mode // App, Terminal or Off, already resolved from Auto
	Theme    string   // "dark" (default), "light", "contrast" or "plain"
	NoLaunch bool     // leave Claude Desktop closed after a switch or a chat change
	// NewerVersion, when set, is called once in the background; it returns a newer released
	// version ("3.0.1") or "" when this one is current or the check is off.
	NewerVersion func() string
}

// Run shows the app until the user quits.
func Run(env *ops.Env, o Options) error { return runApp(env, o) }
