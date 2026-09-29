package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
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

// newTestApp is the app at a fixed size in a language, with its state read.
func newTestApp(t *testing.T, env *ops.Env, lang string, width, height int) *app {
	t.Helper()
	i18n.Load(lang)
	t.Cleanup(func() { i18n.Load("en") })
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

func TestAppFrames(t *testing.T) {
	for _, lang := range []string{"en", "fa"} {
		for _, size := range [][2]int{{80, 24}, {120, 40}} {
			t.Run(fmt.Sprintf("%s_%dx%d", lang, size[0], size[1]), func(t *testing.T) {
				env, _ := fixture(t)
				a := newTestApp(t, env, lang, size[0], size[1])
				checkFrame(t, a)
				l := a.L()
				text := strings.Join(screen(a), "\n")
				for _, want := range []string{
					"work", "personal", "client", "v3.0.0",
					l.Text(i18n.T("ui.badge.active")),
					l.Text(i18n.T("count.chats", "n", 3)),
					l.Text(i18n.T("ui.accounts.saved", "ago", i18n.Ago(now.Add(-2*time.Hour), now))),
					l.Text(i18n.T("ui.accounts.add")),
					l.Text(i18n.T("tab.settings")),
					"0ce4cc0c",
				} {
					if !strings.Contains(text, want) {
						t.Errorf("frame lacks %q", want)
					}
				}
				golden.RequireEqual(t, text)
			})
		}
	}
}

// The header mirrors in Persian: tabs from the right edge in reading order, the pill at the left.
func TestHeaderMirrors(t *testing.T) {
	for _, lang := range []string{"en", "fa"} {
		t.Run(lang, func(t *testing.T) {
			env, _ := fixture(t)
			a := newTestApp(t, env, lang, 120, 40)
			line, tabs := a.header()
			l := a.L()
			for i, s := range a.screens {
				if got, want := ansi.Strip(ansi.Cut(line, tabs[i].x0, tabs[i].x1)), l.Text(s.Title()); got != want {
					t.Errorf("tab %d spans %q, want %q", i, got, want)
				}
			}
			first, last := tabs[0], tabs[len(tabs)-1]
			row := ansi.Strip(line)
			brand := "nyxon " + i18n.T("app.name") // a brand lockup: the same in both languages
			if lang == "en" {
				if first.x0 >= last.x0 || !strings.HasPrefix(row, brand) || strings.HasSuffix(row, " ") {
					t.Errorf("english header should read left to right with the pill at the right edge: %q", row)
				}
				return
			}
			if first.x0 <= last.x0 || !strings.HasSuffix(row, brand) || strings.HasPrefix(row, " ") {
				t.Errorf("persian header should start at the right edge with the pill at the left: %q", row)
			}
			if first.x1 <= a.Width/2 {
				t.Errorf("first tab ends at %d, want it in the right half of %d", first.x1, a.Width)
			}
			if div := ansi.Strip(a.divider(tabs)); []rune(div)[first.x0] != '━' || []rune(div)[first.x0-1] != '─' {
				t.Errorf("the underline should sit under the first tab: %q", div)
			}
		})
	}
}

func TestTabsSwitch(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
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
	a := newTestApp(t, env, "en", 50, 15)
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
	a := newTestApp(t, env, "en", 80, 24)
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

// Focus starts on the profile in use; arrows and clicks follow the cards as drawn, mirrored in
// Persian; clicking the focused card acts on it.
func TestCardsFocusAndClick(t *testing.T) {
	for _, tc := range []struct {
		lang             string
		right, leftClick int // focus after right from work (2), and the index of the leftmost card
	}{{"en", 3, 0}, {"fa", 1, 2}} {
		t.Run(tc.lang, func(t *testing.T) {
			env, _ := fixture(t)
			a := newTestApp(t, env, tc.lang, 120, 40)
			a.View()
			s := a.screens[0].(*accountsScreen)
			if s.focus != 2 {
				t.Fatalf("focus starts on %d, want the profile in use (2)", s.focus)
			}
			press(a, "right")
			if s.focus != tc.right {
				t.Errorf("right moved focus to %d, want %d", s.focus, tc.right)
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
			if s.focus != tc.leftClick {
				t.Errorf("clicking the left card focused %d, want %d", s.focus, tc.leftClick)
			}
			if a.runner != nil {
				t.Fatal("a first click should only focus")
			}
			// In English the left card is client, so it switches; in Persian it is work, in use already.
			click(0)
			if a.runner == nil && a.toast.text != i18n.T("ops.err.already-current", "name", "work") {
				t.Error("clicking the focused card should act on it")
			}
		})
	}
}

// The app looks whether Desktop runs every few seconds and reads the whole state (every chat
// card) only when that changed, when a tab opens, after a change, and once a minute otherwise.
func TestStatusPolling(t *testing.T) {
	env, desk := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
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
