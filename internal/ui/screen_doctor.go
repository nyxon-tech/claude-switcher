package ui

import "github.com/nyxon-tech/claude-switcher/v3/internal/i18n"

// doctorScreen will list ops.Doctor's checks.
type doctorScreen struct{ placeholder }

func (*doctorScreen) Title() string { return i18n.T("tab.doctor") }
