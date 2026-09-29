package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

const (
	workID   = "0ce4cc0c-1111-4111-8111-111111111111"
	personID = "7a3f19b2-2222-4222-8222-222222222222"
	clientID = "5d0e8a41-3333-4333-8333-333333333333"
	orgID    = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

var now = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// fakeDesktop stands in for Claude Desktop: quitting (from the tray, or forced) closes it and
// launching opens it.
type fakeDesktop struct {
	mu                      sync.Mutex
	running                 bool
	quitErr                 error
	quits, forces, launches int
}

func (d *fakeDesktop) Running() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running, nil
}

func (d *fakeDesktop) Quit() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.quits++
	d.running = false
	return d.quitErr
}

func (d *fakeDesktop) ForceQuit() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.forces++
	d.running = false
	return nil
}

func (d *fakeDesktop) Launch() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.launches++
	d.running = true
	return nil
}

func (d *fakeDesktop) StopHelpers() {}

func (d *fakeDesktop) count() (quits, forces, launches int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.quits, d.forces, d.launches
}

var _ platform.Desktop = (*fakeDesktop)(nil)

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func login(account string) string { return `{"lastKnownAccountUuid":"` + account + `"}` }

// fixture is a Desktop data folder signed into "work", a vault with three saved profiles, and
// chat lists for two of the accounts (one chat has a Persian title). Everything is in a temp dir.
func fixture(t *testing.T) (*ops.Env, *fakeDesktop) {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	root := t.TempDir()
	data := filepath.Join(root, "Claude")
	env, err := ops.Open(ops.Options{DataDir: data, VaultDir: filepath.Join(root, "vault"), ProjectsDir: filepath.Join(root, "projects")})
	if err != nil {
		t.Fatal(err)
	}
	desk := &fakeDesktop{}
	env.Desktop = desk
	env.Now = func() time.Time { return now }

	put(t, filepath.Join(data, "config.json"), login(workID))
	put(t, filepath.Join(data, "Preferences"), "work")
	for _, p := range []struct {
		name, account string
		saved         time.Duration
	}{{"work", workID, 2 * time.Hour}, {"personal", personID, 3 * 24 * time.Hour}, {"client", clientID, 45 * time.Minute}} {
		dir := filepath.Join(env.Vault.Dir, p.name)
		put(t, filepath.Join(dir, "config.json"), login(p.account))
		put(t, filepath.Join(dir, "Preferences"), p.name)
		put(t, filepath.Join(dir, "_account"), p.account)
		saved := now.Add(-p.saved)
		if err := os.Chtimes(filepath.Join(dir, "config.json"), saved, saved); err != nil {
			t.Fatal(err)
		}
	}
	put(t, filepath.Join(env.Vault.Dir, "_current_profile"), "work")
	put(t, filepath.Join(env.Vault.Dir, "settings.json"), `{"onboarded":true}`) // the welcome was seen

	chats := map[string][]string{
		workID:   {"Fix the login redirect", "بازنویسی صفحه‌ی ورود", "Release notes"},
		personID: {"Trip plan", "Budget sheet"},
	}
	for account, titles := range chats {
		for i, title := range titles {
			id := fmt.Sprintf("%s-%04d-4000-8000-000000000000", account[:8], i)
			card := fmt.Sprintf(`{"sessionId":"local_%s","cliSessionId":"%s","cwd":"/work/proj","title":%q,"lastActivityAt":%d}`,
				id, id, title, now.Add(-time.Duration(i)*time.Hour).UnixMilli())
			put(t, filepath.Join(data, "claude-code-sessions", account, orgID, "local_"+id+".json"), card)
		}
	}
	return env, desk
}

// newTestApp is the app at a fixed size, with its state read.
func newTestApp(t *testing.T, env *ops.Env, width, height int) *app {
	t.Helper()
	a := newApp(env, Options{Version: "3.0.0", Mode: rtl.App, Theme: "dark"})
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	a.Update(a.Refresh()())
	return a
}

func press(a *app, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		msg := tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "right":
			msg = tea.KeyPressMsg{Code: tea.KeyRight}
		case "left":
			msg = tea.KeyPressMsg{Code: tea.KeyLeft}
		case "ctrl+c":
			msg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
		}
		_, cmd = a.Update(msg)
	}
	return cmd
}

// screen is the frame as the terminal shows it, colours stripped, one string per row.
func screen(a *app) []string { return strings.Split(ansi.Strip(a.View().Content), "\n") }

// checkFrame fails when the frame is not exactly the window: every row within the width.
func checkFrame(t *testing.T, a *app) {
	t.Helper()
	lines := strings.Split(a.View().Content, "\n")
	if len(lines) != a.Height {
		t.Errorf("frame has %d rows, want %d", len(lines), a.Height)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > a.Width {
			t.Errorf("row %d is %d cells wide, window is %d: %q", i, w, a.Width, ansi.Strip(line))
		}
	}
}

// Accounts at every size the goldens keep: the hero when there is room, the cards, the header.
func TestAppFrames(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("en_%dx%d", size[0], size[1]), func(t *testing.T) {
			env, _ := fixture(t)
			a := newTestApp(t, env, size[0], size[1])
			checkFrame(t, a)
			text := strings.Join(screen(a), "\n")
			for _, want := range []string{
				"work", "personal", "client", "v3.0.0", "0ce4cc0c",
				i18n.T("ui.badge.active"), i18n.T("count.chats", "n", 3), i18n.T("ui.accounts.add"), i18n.T("tab.settings"),
				i18n.T("ui.accounts.saved", "ago", i18n.Ago(now.Add(-2*time.Hour), now)), i18n.T("app.tagline"),
			} {
				if !strings.Contains(text, want) {
					t.Errorf("frame lacks %q", want)
				}
			}
			if hero := strings.Contains(text, "C L A U D E   S W I T C H E R"); hero != (size[1] >= 24) {
				t.Errorf("the hero shows: %v; want it only where it fits", hero)
			}
			golden.RequireEqual(t, text)
		})
	}
}

// The header reads left to right: the brand, the tabs, and the status pill at the right edge.
// As the window narrows, "by nyxon" goes first, then the pill's detail, then the brand.
func TestHeader(t *testing.T) {
	pill, by := " "+i18n.T("app.name")+" ", i18n.T("app.by")+" nyxon"
	for _, tc := range []struct {
		width         int
		brand, status string
	}{
		{120, pill + " " + by + "   ", "● work  " + i18n.T("ui.desktop.closed")},
		{100, pill + "   ", "● work  " + i18n.T("ui.desktop.closed")},
		{80, pill + "   ", "● work"},
		{60, "", "● work"},
	} {
		t.Run(fmt.Sprint(tc.width), func(t *testing.T) {
			env, _ := fixture(t)
			a := newTestApp(t, env, tc.width, 30)
			line, tabs := a.header()
			for i, s := range a.screens {
				if got := ansi.Strip(ansi.Cut(line, tabs[i].x0, tabs[i].x1)); got != s.Title() {
					t.Errorf("tab %d spans %q, want %q", i, got, s.Title())
				}
			}
			row := ansi.Strip(line)
			if ansi.StringWidth(line) != tc.width || !strings.HasPrefix(row, tc.brand+a.screens[0].Title()) || !strings.HasSuffix(row, tc.status) {
				t.Errorf("header %q: want %q, the tabs, and %q at the right edge", row, tc.brand, tc.status)
			}
			if div := []rune(ansi.Strip(a.divider(tabs))); div[tabs[0].x0] != '━' || div[tabs[0].x1] != '─' {
				t.Errorf("the underline should sit under the first tab: %q", string(div))
			}
		})
	}
}

// The hero is the brand as a title block, centred: the star, the spaced name, "by nyxon" and the
// tagline; below 40 cells the star and the name share a line. Every line is as wide as asked.
func TestHero(t *testing.T) {
	st := newStyles("dark", true)
	for _, tc := range []struct {
		width int
		title string
	}{{120, "C L A U D E   S W I T C H E R"}, {60, "C L A U D E   S W I T C H E R"}, {39, "✦ Claude Switcher"}} {
		lines := st.hero(Layout{Mode: rtl.App}, tc.width)
		text := ansi.Strip(strings.Join(lines, "\n"))
		for _, want := range []string{tc.title, i18n.T("app.by") + " nyxon", "Switch Claude accounts"} {
			if !strings.Contains(text, want) {
				t.Errorf("%d cells: the hero lacks %q:\n%s", tc.width, want, text)
			}
		}
		for _, line := range lines {
			plain := ansi.Strip(line)
			lead, trail := len(plain)-len(strings.TrimLeft(plain, " ")), len(plain)-len(strings.TrimRight(plain, " "))
			if w := ansi.StringWidth(line); w != tc.width || strings.TrimSpace(plain) != "" && (trail < lead || trail > lead+1) {
				t.Errorf("%d cells: line %q is %d cells, %d and %d cells in", tc.width, plain, w, lead, trail)
			}
		}
	}
	plain := newStyles("plain", true)
	pill := plain.Badge.Render(" " + i18n.T("app.name") + " ")
	if b := plain.brand(true); !plain.Badge.GetReverse() || !plain.Badge.GetBold() || !strings.HasPrefix(b, pill) || strings.Contains(b, "38;2") {
		t.Errorf("the plain theme draws the pill bold in reverse video, and nyxon without colour: %q", b)
	}
}

func TestTabsSwitch(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, 100, 30)
	press(a, "3")
	if a.tab != 2 {
		t.Fatalf("3 should open tab 3, got %d", a.tab)
	}
	press(a, "esc")
	if a.tab != 0 {
		t.Fatalf("esc should go back to Accounts, got tab %d", a.tab)
	}
	_, tabs := a.header()
	a.Update(tea.MouseClickMsg{X: tabs[4].x0 + 1, Y: 0, Button: tea.MouseLeft})
	if a.tab != 4 {
		t.Fatalf("clicking the Doctor tab should open it, got %d", a.tab)
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if a.tab != 3 {
		t.Fatalf("shift+tab should go back one tab, got %d", a.tab)
	}
}

func TestTooSmall(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, 50, 15)
	checkFrame(t, a)
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, i18n.T("ui.too_small")) {
		t.Errorf("a 50x15 window should ask to be larger, got:\n%s", text)
	}
}

func TestEmptyAccounts(t *testing.T) {
	env, _ := fixture(t)
	if err := os.RemoveAll(env.Vault.Dir); err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, env, 80, 24)
	checkFrame(t, a)
	text := strings.Join(screen(a), "\n")
	if !strings.Contains(text, i18n.T("ui.accounts.empty.title")) || !strings.Contains(text, i18n.T("ui.accounts.empty.action")) {
		t.Errorf("no profiles should offer to save the signed-in account:\n%s", text)
	}
	press(a, "s")
	if a.dialog == nil || a.dialog.input == nil || a.dialog.input.Value() != "work" {
		t.Fatalf("s should ask for a name, suggesting work; dialog %+v", a.dialog)
	}
	checkFrame(t, a)
}

// Focus starts on the profile in use; arrows and clicks follow the cards as drawn; clicking the
// focused card acts on it.
func TestCardsFocusAndClick(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, 120, 40)
	a.View()
	s := a.screens[0].(*accountsScreen)
	if s.focus != 2 {
		t.Fatalf("focus starts on %d, want the profile in use (2)", s.focus)
	}
	press(a, "right")
	if s.focus != 3 {
		t.Errorf("right moved focus to %d, want the add card (3)", s.focus)
	}
	if press(a, "left"); s.focus != 2 {
		t.Errorf("left should undo right, focus is %d", s.focus)
	}
	a.View()
	g := s.geo
	click := func(col int) {
		a.Update(tea.MouseClickMsg{X: g.x + col*(g.cardW+gapX) + g.cardW/2, Y: 2 + g.y + 1, Button: tea.MouseLeft})
	}
	click(1)
	if s.focus != 1 {
		t.Errorf("clicking the middle card focused %d, want 1", s.focus)
	}
	click(0)
	if s.focus != 0 || a.runner != nil {
		t.Fatalf("a first click on the left card should only focus it; focus %d", s.focus)
	}
	click(0) // client, so it switches
	if a.runner == nil {
		t.Error("clicking the focused card should act on it")
	}
}

// Persian user data reads right on Accounts, Activity and Usage (Chats has its own tests): laid
// out by the app (rtl.App) its letters are joined and in visual order, left to the terminal
// (rtl.Terminal) it passes through as typed. Either way it keeps to its column: a card, a row
// and a bar stay exactly as wide as they are drawn, whatever name they hold.
func TestPersianData(t *testing.T) {
	const name = "مشتری"                            // a profile
	const long = "حساب کاری شرکت بازاریابی دیجیتال" // too long for any column
	for _, mode := range []rtl.Mode{rtl.App, rtl.Terminal} {
		t.Run(mode.String(), func(t *testing.T) {
			env, _ := fixture(t)
			if err := env.Vault.Rename("client", name); err != nil {
				t.Fatal(err)
			}
			activityFixture(t, env) // a change into a list named خانه
			usageFixture(t, env)    // a project folder named بازاریابی
			a := newTestApp(t, env, 100, 30)
			a.Opts.Mode = mode
			for _, tc := range []struct{ tab, text string }{{"1", name}, {activityTab, "خانه"}, {usageTab, "بازاریابی"}} {
				deliver(a, press(a, tc.tab))
				checkFrame(t, a)
				frame, want := frameText(a), tc.text
				if mode == rtl.App {
					want = rtl.Visual(tc.text, rtl.DirAuto)
				}
				bare := strings.ContainsFunc(frame, func(r rune) bool { return unicode.IsLetter(r) && r >= 0x0600 && r <= 0x06ff })
				if !strings.Contains(frame, want) || bare != (mode == rtl.Terminal) {
					t.Errorf("tab %s should show %q, and bare Arabic letters only in terminal mode (%v):\n%s", tc.tab, want, bare, frame)
				}
			}

			c := a.Ctx
			card := profileCard(c, ops.Profile{Name: long, Current: true, SignedIn: true, Account: workID}, true, maxCardW)
			summary := i18n.T("ops.done.copy", "n", 1, "from", "work", "to", long)
			row := a.screens[2].(*activityScreen).row(c, store.Journal{Summary: summary, At: now}, true, 70)
			for _, tc := range []struct {
				what  string
				lines []string
				width int
			}{
				{"card", strings.Split(card, "\n"), maxCardW},
				{"activity row", []string{row}, 70},
				{"usage bar", shareRows(c, []share{{long, 5}, {"api", 3}}, 8, 50), 50},
			} {
				for _, line := range tc.lines {
					if w := ansi.StringWidth(line); w != tc.width {
						t.Errorf("a %s line is %d cells, want %d: %q", tc.what, w, tc.width, ansi.Strip(line))
					}
				}
			}
		})
	}
}

// The app looks whether Desktop runs every few seconds and reads the whole state (every chat
// card) only when that changed, when a tab opens, after a change, and once a minute otherwise.
func TestStatusPolling(t *testing.T) {
	env, desk := fixture(t)
	a := newTestApp(t, env, 100, 30)
	look := func() { // what the poll finds now
		running, err := desk.Running()
		a.Update(pollMsg{running, err})
	}
	reads := a.reads
	want := func(n int, why string) {
		t.Helper()
		if a.reads != reads+n {
			t.Fatalf("%d reads, want %d: %s", a.reads-reads, n, why)
		}
	}
	look()
	want(0, "Desktop is still closed")
	desk.Launch()
	look()
	want(1, "Desktop started")
	if !a.Status.Running {
		t.Error("the pill should show Desktop running at once")
	}
	for range readEvery - 1 {
		look()
	}
	want(1, "nothing changed for less than a minute")
	look()
	want(2, "a minute passed")
	press(a, "2")
	want(3, "a tab opened")
	a.Update(doneMsg{ok: "done"})
	want(4, "a change was made")
}
