package ui

import (
	"image/color"
	"os"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// palette is one theme's colours. A nil colour leaves the terminal's own (the plain theme).
type palette struct {
	text, muted, dim, accent, line, void, ok, warn, fail color.Color
}

var (
	nyxonDark = palette{
		text: lipgloss.Color("#f3f0e8"), muted: lipgloss.Color("#aaaec4"), dim: lipgloss.Color("#6b7090"),
		accent: lipgloss.Color("#a8b2ff"), line: lipgloss.Color("#303345"), void: lipgloss.Color("#090b14"),
		ok: lipgloss.Color("#7ee2a8"), warn: lipgloss.Color("#f5c97a"), fail: lipgloss.Color("#ff8f8f"),
	}
	nyxonLight = palette{
		text: lipgloss.Color("#1b1e2e"), muted: lipgloss.Color("#5d6178"), dim: lipgloss.Color("#9397ad"),
		accent: lipgloss.Color("#5b5fd6"), line: lipgloss.Color("#d5d7e3"), void: lipgloss.Color("#ffffff"),
		ok: lipgloss.Color("#1e8a57"), warn: lipgloss.Color("#9a6700"), fail: lipgloss.Color("#c73a3a"),
	}
	contrastDark = palette{
		text: lipgloss.Color("#ffffff"), muted: lipgloss.Color("#e4e4e4"), dim: lipgloss.Color("#b4b4b4"),
		accent: lipgloss.Color("#00e5ff"), line: lipgloss.Color("#ffffff"), void: lipgloss.Color("#000000"),
		ok: lipgloss.Color("#00ff87"), warn: lipgloss.Color("#ffd700"), fail: lipgloss.Color("#ff5f5f"),
	}
	contrastLight = palette{
		text: lipgloss.Color("#000000"), muted: lipgloss.Color("#1c1c1c"), dim: lipgloss.Color("#4e4e4e"),
		accent: lipgloss.Color("#0000d7"), line: lipgloss.Color("#000000"), void: lipgloss.Color("#ffffff"),
		ok: lipgloss.Color("#005f00"), warn: lipgloss.Color("#875f00"), fail: lipgloss.Color("#af0000"),
	}
)

// themePalette picks the palette for a theme name (the CLI's short names or the settings file's
// long ones). "" and "auto" follow the terminal background, and so does contrast, which is
// white on dark and black on light.
func themePalette(theme string, dark bool) palette {
	switch theme {
	case "plain":
		return palette{}
	case "light", "nyxon-light":
		return nyxonLight
	case "contrast":
		if dark {
			return contrastDark
		}
		return contrastLight
	case "", "auto":
		if !dark {
			return nyxonLight
		}
	}
	return nyxonDark
}

// Palette is a theme's colours for output outside the app, such as the command line's. A nil
// colour leaves the terminal's own.
type Palette struct{ Text, Muted, Dim, Accent, Line, OK, Warn, Fail color.Color }

// ThemePalette is the palette of a theme ("auto", "dark" or "nyxon-dark", "light" or
// "nyxon-light", "contrast" or "plain") on a dark or a light terminal background.
func ThemePalette(theme string, dark bool) Palette {
	p := themePalette(theme, dark)
	return Palette{p.text, p.muted, p.dim, p.accent, p.line, p.ok, p.warn, p.fail}
}

// effectiveTheme is the theme to draw with: NO_COLOR always means plain.
func effectiveTheme(theme string) string {
	if os.Getenv("NO_COLOR") != "" {
		return "plain"
	}
	return theme
}

// Styles are the looks every screen draws with. Take them from Ctx.St; never build colours in a
// screen, so every theme (plain included) works everywhere.
type Styles struct {
	Text, Muted, Dim, Accent lipgloss.Style
	OK, Warn, Fail           lipgloss.Style
	Bold                     lipgloss.Style // emphasis in the text colour
	Title                    lipgloss.Style // dialog and section titles
	Key                      lipgloss.Style // key names in hints
	Badge                    lipgloss.Style // small label on the accent, such as ACTIVE
	DangerBadge              lipgloss.Style // the focused button of a destructive choice
	TabOn, TabOff            lipgloss.Style
	Bar                      lipgloss.Style // the selection bar at the left edge of a row
	Divider                  lipgloss.Style // lines in the line colour
	Plain                    bool           // no colours: mark focus with bold, reverse and glyphs

	pal palette
}

func newStyles(theme string, dark bool) Styles {
	p := themePalette(theme, dark)
	fg := func(c color.Color) lipgloss.Style {
		if c == nil {
			return lipgloss.NewStyle()
		}
		return lipgloss.NewStyle().Foreground(c)
	}
	s := Styles{
		Text: fg(p.text), Muted: fg(p.muted), Dim: fg(p.dim), Accent: fg(p.accent),
		OK: fg(p.ok), Warn: fg(p.warn), Fail: fg(p.fail),
		Bold: fg(p.text).Bold(true), Title: fg(p.text).Bold(true), Key: fg(p.text).Bold(true),
		TabOn: fg(p.accent).Bold(true), TabOff: fg(p.muted),
		Bar: fg(p.accent), Divider: fg(p.line),
		Plain: p.accent == nil,
		pal:   p,
	}
	if s.Plain {
		s.Badge = lipgloss.NewStyle().Reverse(true).Bold(true)
		s.DangerBadge = s.Badge
		s.TabOn = s.TabOn.Underline(true)
		s.Dim = s.Dim.Faint(true)
	} else {
		s.Badge = lipgloss.NewStyle().Foreground(p.void).Background(p.accent).Bold(true)
		s.DangerBadge = lipgloss.NewStyle().Foreground(p.void).Background(p.fail).Bold(true)
	}
	return s
}

// Box is a rounded panel: line colour normally, accent when focused. The plain theme marks focus
// with a thick border instead.
func (s Styles) Box(focused bool) lipgloss.Style {
	b := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	c := s.pal.line
	if focused {
		c = s.pal.accent
		if s.Plain {
			b = b.Border(lipgloss.ThickBorder())
		}
	}
	if c != nil {
		b = b.BorderForeground(c)
	}
	return b
}

// input styles a text field to match the theme.
func (s Styles) input() textinput.Styles {
	st := textinput.DefaultStyles(true)
	st.Focused.Text, st.Blurred.Text = s.Text, s.Text
	st.Focused.Placeholder, st.Blurred.Placeholder = s.Dim, s.Dim
	st.Focused.Prompt, st.Blurred.Prompt = s.Accent, s.Muted
	if s.pal.accent != nil {
		st.Cursor.Color = s.pal.accent
	}
	return st
}

// Ghost is a Box with a dashed outline, for a card that adds something rather than showing it.
func (s Styles) Ghost(focused bool) lipgloss.Style {
	if focused && s.Plain {
		return s.Box(true)
	}
	dashed := lipgloss.RoundedBorder()
	dashed.Top, dashed.Bottom, dashed.Left, dashed.Right = "╌", "╌", "╎", "╎"
	return s.Box(focused).BorderStyle(dashed)
}
