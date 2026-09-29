package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
)

// placeholder stands in for a screen that is not built yet: a centred muted line. Embed it and
// add Title; delete this file once no screen embeds it.
type placeholder struct{}

func (placeholder) Update(*Ctx, tea.Msg) tea.Cmd { return nil }
func (placeholder) Keys(*Ctx) []key.Binding      { return nil }
func (placeholder) Typing() bool                 { return false }

func (placeholder) View(c *Ctx, width, height int) string {
	l := c.L()
	return middle(l, c.St.Muted.Render(l.Fit(i18n.T("ui.soon"), width)), width, height)
}
