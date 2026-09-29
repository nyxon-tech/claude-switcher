package ui

import (
	"cmp"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

// settingsScreen changes the saved settings, each one the moment it is picked.
type settingsScreen struct {
	focus int
	top   int   // the body line the first setting was drawn on
	hits  []int // the setting each drawn line belongs to, -1 between them, for clicks
}

// setting is one row of the screen. values are what settings.json holds, in the order they
// cycle; a row without values is an action.
type setting struct {
	key    string // settings.<key> is the label and settings.<key>.hint the line under it
	values []string
	names  string // the i18n key of a value's name is names + value
	get    func(store.Settings) string
	set    func(*store.Settings, string)
}

var settingRows = []setting{
	{key: "theme", values: []string{"auto", "nyxon-dark", "nyxon-light", "contrast", "plain"}, names: "settings.theme.",
		get: func(s store.Settings) string { return cmp.Or(s.Theme, "auto") },
		set: func(s *store.Settings, v string) { s.Theme = v }},
	{key: "rtl", values: []string{"auto", "app", "terminal", "off"}, names: "settings.rtl.",
		get: func(s store.Settings) string { return cmp.Or(s.RTL, "auto") },
		set: func(s *store.Settings, v string) { s.RTL = v }},
	{key: "update", values: []string{"on", "off"}, names: "settings.",
		get: func(s store.Settings) string {
			if s.UpdateCheck {
				return "on"
			}
			return "off"
		},
		set: func(s *store.Settings, v string) { s.UpdateCheck = v == "on" }},
	{key: "welcome"},
}

// rtlSample is the chat title the preview draws: Persian letters to join, words to run right to
// left, and brackets to mirror around a number.
const rtlSample = "طراحی صفحه\u200cی داشبورد (نسخه ۲)"

type settingsSavedMsg struct{ err error }

// Apply saves s as the settings and puts a new theme or rtl mode into effect at once; every
// screen then gets restyleMsg. It runs in Update, so no View draws half a change.
func (c *Ctx) Apply(s store.Settings) tea.Cmd {
	old := c.Settings
	c.Settings = s
	var cmds []tea.Cmd
	if s.Theme != old.Theme {
		c.theme = effectiveTheme(s.Theme)
		c.St = newStyles(c.theme, c.dark)
	}
	if s.RTL != old.RTL {
		c.Opts.Mode = rtlMode(s.RTL)
	}
	if s.Theme != old.Theme || s.RTL != old.RTL {
		cmds = append(cmds, func() tea.Msg { return restyleMsg{} })
	}
	if c.settingsErr != nil { // writing would replace what the file holds with the defaults
		return tea.Batch(append(cmds, c.Toast(toastWarn, i18n.T("settings.unsaved", "error", c.settingsErr.Error())))...)
	}
	return tea.Batch(append(cmds, c.saver.save(s))...)
}

// rtlMode is the mode an rtl setting names, auto decided for this terminal.
func rtlMode(setting string) rtl.Mode {
	if m, ok := rtl.ParseMode(setting); ok && m != rtl.Auto {
		return m
	}
	return rtl.Detect(os.Getenv, runtime.GOOS)
}

// settingsWriter writes settings.json in the background. Each write saves the newest settings,
// so writes that finish out of order never leave an older choice on disk.
type settingsWriter struct {
	mu     sync.Mutex
	vault  store.Vault
	newest store.Settings
}

func (w *settingsWriter) save(s store.Settings) tea.Cmd {
	w.mu.Lock()
	w.newest = s
	w.mu.Unlock()
	return func() tea.Msg {
		w.mu.Lock()
		defer w.mu.Unlock()
		return settingsSavedMsg{w.vault.SaveSettings(w.newest)}
	}
}

func (*settingsScreen) Title() string { return i18n.T("tab.settings") }
func (*settingsScreen) Typing() bool  { return false }

func (s *settingsScreen) Keys(*Ctx) []key.Binding {
	keys := []key.Binding{bind("↑↓", "ui.key.move", "up", "down", "k", "j")}
	if settingRows[s.focus].values == nil {
		return append(keys, bind("enter", "settings.key.open", "enter", "space"))
	}
	return append(keys, bind("←→", "settings.key.change", "left", "right", "h", "l", "enter", "space"))
}

func (s *settingsScreen) Update(c *Ctx, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case settingsSavedMsg:
		if msg.err != nil {
			return c.Toast(toastFail, i18n.T("settings.save_failed", "error", msg.err.Error()))
		}
	case tea.KeyPressMsg:
		return s.key(c, msg.String())
	case tea.MouseClickMsg:
		if y := msg.Y - s.top; y >= 0 && y < len(s.hits) && s.hits[y] >= 0 {
			if s.hits[y] == s.focus {
				return s.change(c, 1)
			}
			s.focus = s.hits[y]
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			return s.key(c, "up")
		case tea.MouseWheelDown:
			return s.key(c, "down")
		}
	}
	return nil
}

func (s *settingsScreen) key(c *Ctx, k string) tea.Cmd {
	action := settingRows[s.focus].values == nil
	switch k {
	case "up", "k":
		s.focus = max(0, s.focus-1)
	case "down", "j":
		s.focus = min(len(settingRows)-1, s.focus+1)
	case "right", "l":
		if !action {
			return s.change(c, 1)
		}
	case "left", "h":
		if !action {
			return s.change(c, -1)
		}
	case "enter", "space":
		return s.change(c, 1)
	}
	return nil
}

// change moves the focused setting step values on, or runs its action.
func (s *settingsScreen) change(c *Ctx, step int) tea.Cmd {
	f := settingRows[s.focus]
	if f.values == nil {
		return c.Welcome()
	}
	set := c.Settings
	n := len(f.values)
	i := slices.Index(f.values, f.get(set)) // -1 for a value this version does not know
	f.set(&set, f.values[((i+step)%n+n)%n])
	return c.Apply(set)
}

func (s *settingsScreen) View(c *Ctx, width, height int) string {
	l := c.L()
	labelW, nameW, hintW := 0, 0, 0
	for _, f := range settingRows {
		hintW = max(hintW, l.Mode.Width(i18n.T("settings."+f.key+".hint")))
		if f.values == nil {
			continue
		}
		labelW = max(labelW, l.Mode.Width(i18n.T("settings."+f.key)))
		for _, v := range f.values {
			nameW = max(nameW, l.Mode.Width(i18n.T(f.names+v)))
		}
	}
	// A row is the bar, a space, the label, three spaces and the value between ‹ and › (with a
	// toggle's dot); a hint is indented by two.
	blockW := min(width-4, max(labelW+nameW+11, hintW+2))
	labelW = min(labelW, blockW/2)
	lines := s.lines(c, blockW, labelW, 1)
	if len(lines) > height {
		lines = s.lines(c, blockW, labelW, 0)
	}
	s.top = max(0, (height-len(lines))/2)
	out := make([]string, s.top, height)
	for _, line := range lines {
		out = append(out, l.Center(l.Pad(line, blockW), width))
	}
	return strings.Join(out, "\n")
}

// lines are the settings, each with its hint under it (the rtl one also with its preview), gap
// blank lines apart. It notes which setting each line belongs to.
func (s *settingsScreen) lines(c *Ctx, blockW, labelW, gap int) []string {
	l, st := c.L(), c.St
	var lines []string
	s.hits = s.hits[:0]
	for i, f := range settingRows {
		if i > 0 {
			for range gap {
				lines, s.hits = append(lines, ""), append(s.hits, -1)
			}
		}
		block := []string{
			settingRow(c, f, i == s.focus, labelW, blockW-labelW-5),
			l.Inline("  ", st.Dim.Render(l.Fit(i18n.T("settings."+f.key+".hint"), blockW-2))),
		}
		if f.key == "rtl" {
			for _, line := range preview(c, blockW-2) {
				block = append(block, l.Inline("  ", line))
			}
		}
		for range block {
			s.hits = append(s.hits, i)
		}
		lines = append(lines, block...)
	}
	return lines
}

// settingRow is one setting: the selection bar, its label and its value, between ‹ and › while
// focused, as the arrows change it. An action is its label and a pointer.
func settingRow(c *Ctx, f setting, focused bool, labelW, valueW int) string {
	l, st := c.L(), c.St
	bar, label, value := " ", st.Text, st.Muted
	if focused {
		bar, label, value = st.Bar.Render(l.BarGlyph()), st.Bold, st.Accent
	}
	if f.values == nil {
		text := label.Render(l.Fit(i18n.T("settings."+f.key), labelW+valueW))
		return l.Inline(bar, " ", text, " ", value.Render(l.Pointer()))
	}
	v := f.get(c.Settings)
	name := v // a value this version does not know shows as it is
	if slices.Contains(f.values, v) {
		name = i18n.T(f.names + v)
	}
	shown := value.Render(l.Fit(name, valueW-6))
	if f.names == "settings." { // on or off
		dot := st.Dim.Render("○")
		if v == "on" {
			dot = st.Accent.Render("●")
		}
		shown = l.Inline(dot, " ", shown)
	}
	if focused {
		shown = st.Muted.Render("‹ ") + shown + st.Muted.Render(" ›")
	} else {
		shown = "  " + shown + "  "
	}
	return l.Inline(bar, " ", l.Pad(label.Render(l.Fit(i18n.T("settings."+f.key), labelW)), labelW), "   ", shown)
}

// preview is the sample chat title in a box, drawn the way the rtl setting draws chat titles.
func preview(c *Ctx, width int) []string {
	l, st := c.L(), c.St
	title := st.Text.Render(l.Data(rtlSample, width-4))
	return strings.Split(st.Box(false).Padding(0, 1).Render(title), "\n")
}
