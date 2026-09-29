package cli

import (
	"cmp"
	"image/color"
	"os"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

// palette is one theme's colours, the same as the app's. A nil colour leaves the terminal's own.
type palette struct {
	text, muted, dim, accent, line, ok, warn, fail color.Color
}

var (
	nyxonDark = palette{
		text: lipgloss.Color("#f3f0e8"), muted: lipgloss.Color("#aaaec4"), dim: lipgloss.Color("#6b7090"),
		accent: lipgloss.Color("#a8b2ff"), line: lipgloss.Color("#303345"),
		ok: lipgloss.Color("#7ee2a8"), warn: lipgloss.Color("#f5c97a"), fail: lipgloss.Color("#ff8f8f"),
	}
	nyxonLight = palette{
		text: lipgloss.Color("#1b1e2e"), muted: lipgloss.Color("#5d6178"), dim: lipgloss.Color("#9397ad"),
		accent: lipgloss.Color("#5b5fd6"), line: lipgloss.Color("#d5d7e3"),
		ok: lipgloss.Color("#1e8a57"), warn: lipgloss.Color("#9a6700"), fail: lipgloss.Color("#c73a3a"),
	}
	contrastDark = palette{
		text: lipgloss.Color("#ffffff"), muted: lipgloss.Color("#e4e4e4"), dim: lipgloss.Color("#b4b4b4"),
		accent: lipgloss.Color("#00e5ff"), line: lipgloss.Color("#ffffff"),
		ok: lipgloss.Color("#00ff87"), warn: lipgloss.Color("#ffd700"), fail: lipgloss.Color("#ff5f5f"),
	}
	contrastLight = palette{
		text: lipgloss.Color("#000000"), muted: lipgloss.Color("#1c1c1c"), dim: lipgloss.Color("#4e4e4e"),
		accent: lipgloss.Color("#0000d7"), line: lipgloss.Color("#000000"),
		ok: lipgloss.Color("#005f00"), warn: lipgloss.Color("#875f00"), fail: lipgloss.Color("#af0000"),
	}
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

func paletteFor(theme string, dark bool) palette {
	switch {
	case theme == "plain":
		return palette{}
	case theme == "contrast" && dark:
		return contrastDark
	case theme == "contrast":
		return contrastLight
	case theme == "light" || theme == "auto" && !dark:
		return nyxonLight
	}
	return nyxonDark
}

// styles are the looks of the command line's output.
type styles struct {
	pal                                palette
	muted, dim, accent, ok, warn, fail lipgloss.Style
	plain, bold, border                lipgloss.Style
}

func newStyles(p palette) styles {
	fg := func(c color.Color) lipgloss.Style {
		if c == nil {
			return lipgloss.NewStyle()
		}
		return lipgloss.NewStyle().Foreground(c)
	}
	s := styles{pal: p, muted: fg(p.muted), dim: fg(p.dim), accent: fg(p.accent), ok: fg(p.ok), warn: fg(p.warn),
		fail: fg(p.fail), plain: lipgloss.NewStyle(), bold: fg(p.text).Bold(true), border: fg(p.line)}
	if p.accent == nil {
		s.dim = s.dim.Faint(true) // the plain theme still tells dim text apart
	}
	return s
}

// helpColors dresses Fang's help in the palette the output uses.
func (a *app) helpColors(lipgloss.LightDarkFunc) fang.ColorScheme {
	p := a.st.pal
	if p.accent == nil {
		return fang.ColorScheme{Codeblock: lipgloss.NoColor{}}
	}
	return fang.ColorScheme{
		Base: p.text, Title: p.accent, Description: p.text, Codeblock: lipgloss.NoColor{},
		Program: p.accent, Command: p.accent, DimmedArgument: p.dim, Comment: p.dim, Dash: p.dim,
		Flag: p.ok, FlagDefault: p.muted, QuotedString: p.warn, Argument: p.text, Help: p.text,
	}
}
