package cli

import (
	"cmp"
	"image/color"
	"os"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/nyxon-tech/claude-switcher/v3/internal/ui"
)

// themeNames are the themes the command line takes; settings.json keeps the first two as
// nyxon-dark and nyxon-light.
var themeNames = []string{"auto", "dark", "light", "contrast", "plain"}

func shortTheme(s string) string {
	switch s {
	case "nyxon-dark":
		return "dark"
	case "nyxon-light":
		return "light"
	}
	return cmp.Or(s, "auto")
}

func longTheme(s string) string {
	switch s {
	case "dark":
		return "nyxon-dark"
	case "light":
		return "nyxon-light"
	}
	return s
}

// theme is the theme to draw with: --theme, then the settings; NO_COLOR always means plain.
func (a *app) theme() string {
	if os.Getenv("NO_COLOR") != "" {
		return "plain"
	}
	return shortTheme(cmp.Or(a.flags.theme, a.settings.Theme))
}

// darkBackground asks the terminal, only for the themes that follow it and only when the
// answer shows: JSON has no colours.
func (a *app) darkBackground() bool {
	switch a.theme() {
	case "auto", "contrast":
		in, okIn := a.in.(term.File)
		out, okOut := a.stdout.(term.File)
		if a.tty && a.ttyIn && okIn && okOut && !a.flags.json {
			return lipgloss.HasDarkBackground(in, out)
		}
	case "light":
		return false
	}
	return true
}

// styles are the looks of the command line's output.
type styles struct {
	pal                                ui.Palette
	muted, dim, accent, ok, warn, fail lipgloss.Style
	plain, bold, border                lipgloss.Style
}

func newStyles(p ui.Palette) styles {
	fg := func(c color.Color) lipgloss.Style {
		if c == nil {
			return lipgloss.NewStyle()
		}
		return lipgloss.NewStyle().Foreground(c)
	}
	s := styles{pal: p, muted: fg(p.Muted), dim: fg(p.Dim), accent: fg(p.Accent), ok: fg(p.OK), warn: fg(p.Warn),
		fail: fg(p.Fail), plain: lipgloss.NewStyle(), bold: fg(p.Text).Bold(true), border: fg(p.Line)}
	if p.Accent == nil {
		s.dim = s.dim.Faint(true) // the plain theme still tells dim text apart
	}
	return s
}

// helpColors dresses Fang's help in the palette the output uses.
func (a *app) helpColors(lipgloss.LightDarkFunc) fang.ColorScheme {
	p := a.st.pal
	if p.Accent == nil {
		return fang.ColorScheme{Codeblock: lipgloss.NoColor{}}
	}
	return fang.ColorScheme{
		Base: p.Text, Title: p.Accent, Description: p.Text, Codeblock: lipgloss.NoColor{},
		Program: p.Accent, Command: p.Accent, DimmedArgument: p.Dim, Comment: p.Dim, Dash: p.Dim,
		Flag: p.OK, FlagDefault: p.Muted, QuotedString: p.Warn, Argument: p.Text, Help: p.Text,
	}
}
