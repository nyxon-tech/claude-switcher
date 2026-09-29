package ui

import "github.com/nyxon-tech/claude-switcher/v3/internal/i18n"

// settingsScreen will change the saved settings live.
type settingsScreen struct{ placeholder }

func (*settingsScreen) Title() string { return i18n.T("tab.settings") }
