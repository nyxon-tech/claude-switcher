package ui

import "github.com/nyxon-tech/claude-switcher/v3/internal/i18n"

// activityScreen will be the timeline of changes, with undo.
type activityScreen struct{ placeholder }

func (*activityScreen) Title() string { return i18n.T("tab.activity") }
