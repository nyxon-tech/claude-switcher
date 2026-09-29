// Package ui is the full-screen app: a frame (header with the brand, the tabs and the status
// pill, divider, body, toast line, footer with key hints) around one Screen per tab, plus the
// dialog and the change runner that every screen shares.
//
// Adding a screen: write a type that implements Screen in its own file (screen_<name>.go) and
// list it in newApp. The screen draws its body in View at the size it is given, through c.L()
// so that user data in Persian, Arabic or Hebrew reads right, with colours from c.St. It gets
// keys and clicks only while its tab is open (mouse Y counts from the top of the body), every
// other message always (so a background result finds it on any tab), and shownMsg each time its
// tab opens. It reads state from c.Status or through ops in a tea.Cmd, never in Update or View.
// It writes through c.Change (the change runner closes Claude Desktop around the write) or c.Do,
// and asks through c.Confirm, c.Prompt, c.Choose and c.Alert. A tea.Cmd runs on another
// goroutine, so it takes copies of what it needs from c, never c itself: Settings changes c's
// theme and rtl mode while the app runs, and then every screen gets restyleMsg to rebuild
// anything it made from c.St or c.L().
package ui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

// Screen is one tab of the app.
type Screen interface {
	Title() string                         // the tab label
	Update(c *Ctx, msg tea.Msg) tea.Cmd    // see the package comment for what arrives when
	View(c *Ctx, width, height int) string // the body; short lines line up at the left edge
	Keys(c *Ctx) []key.Binding             // the footer hints, in order; they win over the app's keys
	Typing() bool                          // a text field has focus: q, digits, tab and esc go to the screen
}

// shownMsg is sent to a screen each time its tab opens.
type shownMsg struct{}

// restyleMsg is sent to every screen after the theme or the rtl mode changed.
type restyleMsg struct{}

// Ctx is what every screen shares: the data layer, the options, the styles, the last known
// state, and the dialog, runner and toast of the frame.
type Ctx struct {
	Env      *ops.Env
	Opts     Options
	St       Styles
	Settings store.Settings // as saved; change them through Apply
	Status   ops.Status     // read when a tab opens, after changes, as Desktop starts or quits, each minute
	Loaded   bool           // Status has been read at least once
	Err      error          // the last Status read failed
	Width    int            // the window
	Height   int

	theme       string
	dark        bool  // the terminal's background, dark until it answers otherwise
	settingsErr error // settings.json could not be read, so it is never written over
	saver       *settingsWriter
	onboard     *onboarding // the welcome, while it shows
	dialog      *dialog
	runner      *runner
	toast       toast
	reads, seen int // Status reads started, and the newest shown: a slow read never hides a newer one
	polls       int // looks at Desktop since the last Status read started
}

type toastKind int

const (
	toastInfo toastKind = iota
	toastOK
	toastWarn
	toastFail
)

type toast struct {
	id   int
	kind toastKind
	text string
}

type (
	statusMsg struct {
		read int
		st   ops.Status
		err  error
	}
	pollMsg struct {
		running bool
		err     error
	}
	toastExpiredMsg struct{ id int }
	doneMsg         struct {
		ok  string
		err error
	}
	newerMsg string
)

const (
	pollEvery = 3 * time.Second // how often the app looks whether Desktop runs
	readEvery = 20              // looks between Status reads while nothing changes: a minute
	toastFor  = 4 * time.Second
)

// L is the layout for the current rtl mode.
func (c *Ctx) L() Layout { return Layout{Mode: c.Opts.Mode} }

// Now is the clock ops uses, so tests can fix it.
func (c *Ctx) Now() time.Time { return c.Env.Now() }

// Refresh reads ops.Status in the background.
func (c *Ctx) Refresh() tea.Cmd {
	env := c.Env
	c.reads, c.polls = c.reads+1, 0
	read := c.reads
	return func() tea.Msg {
		st, err := env.Status()
		return statusMsg{read, st, err}
	}
}

// Toast shows a one-line notice above the footer for a few seconds.
func (c *Ctx) Toast(kind toastKind, text string) tea.Cmd {
	text = strings.Join(strings.Fields(text), " ") // an error from the OS may span lines
	c.toast = toast{id: c.toast.id + 1, kind: kind, text: text}
	id := c.toast.id
	return tea.Tick(toastFor, func(time.Time) tea.Msg { return toastExpiredMsg{id} })
}

// Do runs a quick change that leaves Claude Desktop alone (rename, remove) in the background,
// then toasts ok or the error and refreshes the state.
func (c *Ctx) Do(fn func() error, ok string) tea.Cmd {
	return func() tea.Msg { return doneMsg{ok, fn()} }
}

// poll looks whether Desktop runs, a moment from now. A Status read opens every chat card, so
// the app reads it again only when this changes, or after readEvery looks.
func (c *Ctx) poll() tea.Cmd {
	desk := c.Env.Desktop
	return tea.Tick(pollEvery, func(time.Time) tea.Msg {
		running, err := desk.Running()
		return pollMsg{running, err}
	})
}

// app is the Bubble Tea model: the frame and the routing between screens and overlays.
type app struct {
	*Ctx
	screens []Screen
	tab     int
	help    bool
	newer   string
}

func runApp(env *ops.Env, o Options) error {
	_, err := tea.NewProgram(newApp(env, o)).Run()
	return err
}

func newApp(env *ops.Env, o Options) *app {
	theme := effectiveTheme(o.Theme)
	c := &Ctx{Env: env, Opts: o, theme: theme, dark: true, St: newStyles(theme, true), saver: &settingsWriter{vault: env.Vault}}
	c.Settings, c.settingsErr = env.Vault.Settings()
	return &app{Ctx: c, screens: []Screen{
		&accountsScreen{}, &chatsScreen{}, &activityScreen{}, &usageScreen{}, &doctorScreen{}, &settingsScreen{},
	}}
}

func (a *app) Init() tea.Cmd {
	// The background is asked for whatever the theme: Settings can switch to one that follows it.
	cmds := []tea.Cmd{a.Refresh(), a.poll(), a.screens[0].Update(a.Ctx, shownMsg{}), tea.RequestBackgroundColor}
	if a.settingsErr == nil && !a.Settings.Onboarded { // the first run
		a.Welcome()
	}
	if f := a.Opts.NewerVersion; f != nil {
		cmds = append(cmds, func() tea.Msg { return newerMsg(f()) })
	}
	return tea.Batch(cmds...)
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return a, a.key(msg)
	case tea.MouseMsg:
		return a, a.mouse(msg)
	case tea.WindowSizeMsg:
		a.Width, a.Height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		a.dark = msg.IsDark()
		a.St = newStyles(a.theme, a.dark)
	case statusMsg:
		if msg.read < a.seen {
			return a, nil
		}
		a.seen, a.Loaded, a.Err = msg.read, true, msg.err
		if msg.err == nil {
			a.Status = msg.st
		}
	case pollMsg:
		a.polls++
		cmds := []tea.Cmd{a.poll()}
		if msg.err == nil && a.Loaded && msg.running != a.Status.Running {
			a.Status.Running = msg.running // the pill follows at once; the read fills in the rest
			a.polls = readEvery
		}
		if a.polls >= readEvery {
			cmds = append(cmds, a.Refresh())
		}
		return a, tea.Batch(cmds...)
	case toastExpiredMsg:
		if msg.id == a.toast.id {
			a.toast = toast{}
		}
		return a, nil
	case newerMsg:
		a.newer = string(msg)
		return a, nil
	case doneMsg:
		kind, text := toastOK, msg.ok
		if msg.err != nil {
			kind, text = toastFail, msg.err.Error()
		}
		return a, tea.Batch(a.Toast(kind, text), a.Refresh())
	}
	// Everything else reaches the overlays and every screen; each ignores what is not its own.
	var cmds []tea.Cmd
	if a.runner != nil {
		cmds = append(cmds, a.runner.update(a.Ctx, msg))
	}
	if d := a.dialog; d != nil && d.input != nil {
		in, cmd := d.input.Update(msg)
		d.input = &in
		cmds = append(cmds, cmd)
	}
	if a.onboard != nil {
		cmds = append(cmds, a.onboard.update(a, msg))
	}
	for _, s := range a.screens {
		cmds = append(cmds, s.Update(a.Ctx, msg))
	}
	return a, tea.Batch(cmds...)
}

func (a *app) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	switch {
	case k == "ctrl+c" && a.runner != nil && a.runner.phase == running:
		// Quitting now would end the program halfway through writing Claude's files.
		return a.Toast(toastWarn, i18n.T("ui.run.busy"))
	case k == "ctrl+c":
		return tea.Quit
	case a.dialog != nil:
		return a.dialog.update(a.Ctx, msg)
	case a.runner != nil:
		return a.runner.update(a.Ctx, msg)
	case a.tooSmall():
		if k == "q" {
			return tea.Quit
		}
		return nil
	case a.onboard != nil:
		return a.onboard.key(a, msg)
	}
	s := a.screens[a.tab]
	if s.Typing() || key.Matches(msg, s.Keys(a.Ctx)...) {
		return s.Update(a.Ctx, msg)
	}
	switch k {
	case "q":
		return tea.Quit
	case "?":
		a.help = !a.help
	case "tab":
		return a.show(a.tab + 1)
	case "shift+tab":
		return a.show(a.tab - 1)
	case "1", "2", "3", "4", "5", "6":
		return a.show(int(k[0] - '1'))
	case "esc":
		if a.help {
			a.help = false
		} else if a.tab != 0 {
			return a.show(0)
		}
	default:
		return s.Update(a.Ctx, msg)
	}
	return nil
}

// show opens tab i (wrapping around), with the state read afresh.
func (a *app) show(i int) tea.Cmd {
	n := len(a.screens)
	a.tab = (i%n + n) % n
	return tea.Batch(a.screens[a.tab].Update(a.Ctx, shownMsg{}), a.Refresh())
}

func (a *app) mouse(msg tea.MouseMsg) tea.Cmd {
	if a.dialog != nil || a.runner != nil || a.onboard != nil || a.tooSmall() {
		return nil
	}
	m := msg.Mouse()
	_, click := msg.(tea.MouseClickMsg)
	if click && m.Y == 0 {
		_, tabs := a.header()
		for i, t := range tabs {
			if m.X >= t.x0 && m.X < t.x1 {
				return a.show(i)
			}
		}
		return nil
	}
	top, height := 2, a.bodyHeight()
	if m.Y < top || m.Y >= top+height {
		return nil
	}
	m.Y -= top
	switch msg.(type) {
	case tea.MouseClickMsg:
		return a.screens[a.tab].Update(a.Ctx, tea.MouseClickMsg(m))
	case tea.MouseWheelMsg:
		return a.screens[a.tab].Update(a.Ctx, tea.MouseWheelMsg(m))
	}
	return nil
}

func (a *app) tooSmall() bool { return a.Width < 60 || a.Height < 18 }

// bodyHeight is what is left between the divider and the toast line.
func (a *app) bodyHeight() int { return max(0, a.Height-3-len(a.footer())) }

func (a *app) View() tea.View {
	v := tea.NewView(a.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = i18n.T("app.name")
	return v
}

func (a *app) render() string {
	w, h := a.Width, a.Height
	if w <= 0 || h <= 0 {
		return ""
	}
	l := a.L()
	if a.tooSmall() {
		return strings.Join(l.Lines(middle(l, a.St.Muted.Render(l.Fit(i18n.T("ui.too_small"), w)), w, h), w, h), "\n")
	}
	if a.onboard != nil {
		return strings.Join(l.Lines(a.onboard.view(a.Ctx, w, h), w, h), "\n")
	}
	head, tabs := a.header()
	foot := a.footer()
	bodyH := max(0, h-3-len(foot))
	lines := []string{head, a.divider(tabs)}
	body := l.Lines(a.screens[a.tab].View(a.Ctx, w, bodyH), w, bodyH)
	if a.runner != nil || a.dialog != nil {
		for i, line := range body { // dim the screen behind the box
			body[i] = a.St.Dim.Render(ansi.Strip(line))
		}
	}
	lines = append(lines, body...)
	lines = append(lines, a.toastLine())
	lines = append(lines, foot...)
	frame := strings.Join(lines, "\n")

	// Only the top box shows: a question the runner asks covers the runner.
	var box string
	switch {
	case a.dialog != nil:
		box = a.dialog.view(a.Ctx, w, h-2)
	case a.runner != nil:
		box = a.runner.view(a.Ctx, w, h-2)
	default:
		return frame
	}
	ring := 1 // a clear ring keeps the body off the border, when there is room for it
	if lipgloss.Height(box)+2 > h {
		ring = 0
	}
	box = lipgloss.NewStyle().Margin(ring, 2).Render(box)
	x, y := place(box, w, bodyH)
	y += 2
	if lipgloss.Height(box) > bodyH { // a tall box in a short window may cover the frame too
		_, y = place(box, w, h)
	}
	return lipgloss.NewCanvas(w, h).Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(frame), lipgloss.NewLayer(box).X(x).Y(y).Z(1))).Render()
}

// span is where a tab landed on the header line: cells x0 up to x1.
type span struct{ x0, x1 int }

// header draws the brand, the tabs and the status pill on one line. It drops detail until
// everything fits, and returns where each tab landed, for clicks and the underline.
func (a *app) header() (string, []span) {
	l, st, w := a.L(), a.St, a.Width
	full, short := st.brand(true), st.brand(false)
	variants := []struct {
		brand, sep string
		fullPill   bool
	}{
		{full, " · ", true}, {short, " · ", true}, {short, " · ", false},
		{short, "  ", false}, {"", "  ", false}, {"", " ", false},
	}
	for n, v := range variants {
		lead, tabs := "", make([]span, len(a.screens))
		if v.brand != "" {
			lead = v.brand + "   "
		}
		for i, s := range a.screens {
			if i > 0 {
				lead += st.Dim.Render(v.sep)
			}
			style := st.TabOff
			if i == a.tab {
				style = st.TabOn
			}
			x, title := ansi.StringWidth(lead), style.Render(l.Text(s.Title()))
			tabs[i] = span{x, x + ansi.StringWidth(title)}
			lead += title
		}
		pill := a.pill(v.fullPill)
		if ansi.StringWidth(lead)+2+ansi.StringWidth(pill) > w && n < len(variants)-1 {
			continue
		}
		return l.Row(lead, pill, w), tabs
	}
	return "", nil
}

// pill is the status at the right end of the header: the profile in use (● in the ok colour
// when Desktop is signed into it, the warn colour when not, or not saved yet) and whether
// Desktop runs.
func (a *app) pill(full bool) string {
	if !a.Loaded {
		return ""
	}
	l, st := a.L(), a.St
	name := a.Status.Current
	pill := st.Dim.Render("○") + " " + st.Muted.Render(l.Text(i18n.T("ui.pill.none")))
	if name != "" {
		dot := st.Warn.Render("●")
		if p, ok := findProfile(a.Status, name); ok && p.SignedIn {
			dot = st.OK.Render("●")
		}
		pill = dot + " " + st.Text.Render(l.Data(name, 20))
	}
	if full {
		desk := "ui.desktop.closed"
		if a.Status.Running {
			desk = "ui.desktop.running"
		}
		pill += "  " + st.Muted.Render(l.Text(i18n.T(desk)))
	}
	return pill
}

// divider is the line under the header, with the active tab underlined in the accent.
func (a *app) divider(tabs []span) string {
	w, st := a.Width, a.St
	t := tabs[a.tab]
	x0, x1 := min(max(t.x0, 0), w), min(max(t.x1, 0), w)
	return st.Divider.Render(strings.Repeat("─", x0)) + st.Accent.Render(strings.Repeat("━", x1-x0)) +
		st.Divider.Render(strings.Repeat("─", w-x1))
}

// toastLine shows the current toast at the right edge.
func (a *app) toastLine() string {
	l, st, w := a.L(), a.St, a.Width
	if a.toast.text == "" {
		return strings.Repeat(" ", w)
	}
	icon := map[toastKind]string{
		toastInfo: st.Accent.Render("i"), toastOK: st.OK.Render("✓"),
		toastWarn: st.Warn.Render("!"), toastFail: st.Fail.Render("✗"),
	}[a.toast.kind]
	text := st.Text.Render(l.Fit(a.toast.text, w-4))
	return l.End(icon+" "+text+" ", w)
}

// footer is the key hints at the left edge and the version at the right; with "?" every key of
// the screen and the app, in columns, above that line.
func (a *app) footer() []string {
	l, st, w := a.L(), a.St, a.Width
	version := a.Opts.Version
	if version != "" && version[0] >= '0' && version[0] <= '9' { // "3.0.0", not a "dev" build
		version = "v" + version
	}
	version = st.Dim.Render(l.Path(version, 16))
	if a.newer != "" {
		version += "  " + st.Accent.Render(l.Text(i18n.T("ui.update", "version", a.newer)))
	}
	if a.dialog != nil || a.runner != nil {
		return []string{l.End(version, w)}
	}
	keys := a.screens[a.tab].Keys(a.Ctx)
	if !a.help {
		keys = append(keys, bind("?", "ui.key.more"))
		hints := hintLine(a.Ctx, keys, max(0, w-ansi.StringWidth(version)-2))
		return []string{l.Row(hints, version, w)}
	}
	keys = append(keys,
		bind("1-6", "ui.key.tabs"), bind("tab", "ui.key.next_tab"), bind("esc", "ui.key.back"), bind("q", "ui.key.quit"))
	lines := helpGrid(a.Ctx, keys, w)
	lines = lines[:min(len(lines), max(0, a.Height-6))] // the body keeps a few rows
	hints := hintLine(a.Ctx, []key.Binding{bind("?", "ui.key.less")}, w/2)
	return append(lines, l.Row(hints, version, w))
}

// bind is a key binding with its hint. keys are what it matches (label when none are given);
// label and the i18n key desc are what the hints show.
func bind(label, desc string, keys ...string) key.Binding {
	if len(keys) == 0 {
		keys = []string{label}
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(label, i18n.T(desc)))
}

func hintItem(c *Ctx, b key.Binding) string {
	l, st, h := c.L(), c.St, b.Help()
	return st.Key.Render(h.Key) + " " + st.Muted.Render(l.Text(h.Desc))
}

// hintLine is key hints in order, width cells wide. Hints that do not fit are dropped from the
// end, except the last one ("? more").
func hintLine(c *Ctx, keys []key.Binding, width int) string {
	var items []string
	for _, b := range keys {
		if b.Enabled() && b.Help().Key != "" {
			items = append(items, hintItem(c, b))
		}
	}
	for {
		line := strings.Join(items, "  ")
		if ansi.StringWidth(line) <= width || len(items) <= 1 {
			return c.L().Pad(line, width)
		}
		items = append(items[:len(items)-2], items[len(items)-1])
	}
}

// helpGrid lays every hint out in columns, filled top to bottom.
func helpGrid(c *Ctx, keys []key.Binding, width int) []string {
	l := c.L()
	var items []string
	colW := 0
	for _, b := range keys {
		it := hintItem(c, b)
		items = append(items, it)
		colW = max(colW, ansi.StringWidth(it)+3)
	}
	cols := max(1, min(len(items), width/max(colW, 1)))
	rows := (len(items) + cols - 1) / cols
	lines := make([]string, rows)
	for r := range rows {
		line := ""
		for col := range cols {
			if i := col*rows + r; i < len(items) {
				line += l.Pad(items[i], colW)
			}
		}
		lines[r] = l.Pad(line, width)
	}
	return lines
}

// findProfile finds a saved profile by name, ignoring case as the vault does.
func findProfile(st ops.Status, name string) (ops.Profile, bool) {
	for _, p := range st.Profiles {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return ops.Profile{}, false
}
