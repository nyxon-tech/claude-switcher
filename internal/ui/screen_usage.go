package ui

import "github.com/nyxon-tech/claude-switcher/v3/internal/i18n"

// usageScreen will be the token dashboard.
type usageScreen struct{ placeholder }

func (*usageScreen) Title() string { return i18n.T("tab.usage") }
