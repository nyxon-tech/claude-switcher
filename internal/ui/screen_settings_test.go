package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

// settle runs cmd and every command it batches, and feeds a the messages that come back within
// a moment, then runs what those return. Ticks that take longer (toasts, polls) are left out.
func settle(a *app, cmd tea.Cmd) {
	msgs := make(chan tea.Msg, 100)
	start := func(cmd tea.Cmd) {
		if cmd != nil {
			go func() { msgs <- cmd() }()
		}
	}
	start(cmd)
	for {
		select {
		case msg := <-msgs:
			switch msg := msg.(type) {
			case tea.BatchMsg:
				for _, c := range msg {
					start(c)
				}
			case nil:
			default:
				_, next := a.Update(msg)
				start(next)
			}
		case <-time.After(200 * time.Millisecond):
			return
		}
	}
}

// savedSettings is settings.json as a map, unknown keys included.
func savedSettings(t *testing.T, a *app) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(read(t, filepath.Join(a.Env.Vault.Dir, "settings.json"))), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// sgr is how a frame asks the terminal for foreground colour c.
func sgr(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestSettingsFrames(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("en_%dx%d", size[0], size[1]), func(t *testing.T) {
			env, _ := fixture(t)
			a := newTestApp(t, env, "en", size[0], size[1])
			press(a, "6")
			checkFrame(t, a)
			text := strings.Join(screen(a), "\n")
			for _, want := range []string{
				i18n.T("settings.theme"), i18n.T("settings.theme.auto"), i18n.T("settings.rtl"),
				i18n.T("settings.rtl.hint"), i18n.T("settings.update"), i18n.T("settings.welcome"),
				rtl.App.Line(rtlSample, rtl.DirAuto), // the preview: joined, right to left
			} {
				if !strings.Contains(text, want) {
					t.Errorf("frame lacks %q", want)
				}
			}
			golden.RequireEqual(t, text)
		})
	}
}

// Each theme is saved and drawn at once, and keys this version does not know stay in the file.
func TestSettingsTheme(t *testing.T) {
	env, _ := fixture(t)
	put(t, filepath.Join(env.Vault.Dir, "settings.json"), `{"onboarded":true,"fromTheFuture":7}`)
	a := newTestApp(t, env, "en", 100, 30)
	press(a, "6")
	for _, want := range []string{"nyxon-dark", "nyxon-light", "contrast", "plain", "auto"} {
		settle(a, press(a, "right"))
		if a.Settings.Theme != want || a.theme != want {
			t.Fatalf("theme %q (drawn %q), want %q", a.Settings.Theme, a.theme, want)
		}
		if m := savedSettings(t, a); m["theme"] != want || m["fromTheFuture"] != float64(7) {
			t.Errorf("settings.json is %v, want theme %s and the unknown key kept", m, want)
		}
		frame := a.View().Content
		if want == "plain" {
			if !a.St.Plain || strings.Contains(frame, "\x1b[38") {
				t.Error("the plain theme should draw without colours")
			}
			continue
		}
		if accent := sgr(themePalette(want, true).accent); !strings.Contains(frame, accent) {
			t.Errorf("the %s frame should draw with its accent (%s)", want, accent)
		}
		checkFrame(t, a)
	}
}

// The rtl setting changes how chat titles are drawn, and the preview shows it at once.
func TestSettingsRTL(t *testing.T) {
	t.Setenv("CLAUDE_SWITCHER_RTL", "terminal") // what auto finds in this terminal
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	press(a, "6", "down")
	app, terminal := rtl.App.Line(rtlSample, rtl.DirAuto), rtl.Terminal.Line(rtlSample, rtl.DirAuto)
	if app == terminal {
		t.Fatal("the sample should read differently when the app arranges it")
	}
	for _, tc := range []struct {
		setting string
		mode    rtl.Mode
		shown   string
	}{
		{"app", rtl.App, app}, {"terminal", rtl.Terminal, terminal}, {"off", rtl.Off, terminal}, {"auto", rtl.Terminal, terminal},
	} {
		settle(a, press(a, "right"))
		if a.Settings.RTL != tc.setting || a.Opts.Mode != tc.mode {
			t.Fatalf("rtl %q mode %v, want %q and %v", a.Settings.RTL, a.Opts.Mode, tc.setting, tc.mode)
		}
		if got := savedSettings(t, a)["rtl"]; got != tc.setting {
			t.Errorf("settings.json rtl is %v, want %s", got, tc.setting)
		}
		text := strings.Join(screen(a), "\n")
		if !strings.Contains(text, tc.shown) || tc.shown == terminal && strings.Contains(text, app) {
			t.Errorf("%s: the preview should show the sample drawn in %v mode:\n%s", tc.setting, tc.mode, text)
		}
		checkFrame(t, a)
	}
}

func TestSettingsUpdateCheck(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	press(a, "6", "down", "down")
	settle(a, press(a, "enter"))
	if a.Settings.UpdateCheck || savedSettings(t, a)["updateCheck"] != false {
		t.Error("enter should turn the update check off and save it")
	}
}

// The last row opens the welcome again.
func TestSettingsShowsTheWelcome(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	press(a, "6")
	for range settingRows {
		press(a, "down")
	}
	press(a, "enter")
	if a.onboard == nil || a.onboard.saving {
		t.Fatal("enter on the last row should open the welcome at its greeting")
	}
}

// Clicking a setting (its hint and preview too) focuses it; clicking the focused one changes it.
func TestSettingsClick(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	press(a, "6")
	a.View()
	s := a.screens[5].(*settingsScreen)
	click := func(setting int, last bool) tea.Cmd { // on the setting's first or last line
		y := slices.Index(s.hits, setting)
		for last && y+1 < len(s.hits) && s.hits[y+1] == setting {
			y++
		}
		_, cmd := a.Update(tea.MouseClickMsg{X: a.Width / 2, Y: 2 + s.top + y, Button: tea.MouseLeft})
		return cmd
	}
	click(1, true)
	if s.focus != 1 {
		t.Fatalf("clicking the preview focused %d, want the rtl setting", s.focus)
	}
	click(2, false)
	if s.focus != 2 || !a.Settings.UpdateCheck {
		t.Fatalf("a first click should only focus; focus %d", s.focus)
	}
	settle(a, click(2, false))
	if a.Settings.UpdateCheck {
		t.Error("clicking the focused setting should change it")
	}
}

// When settings.json cannot be read, a change still applies but never writes over the file.
func TestSettingsUnreadableFile(t *testing.T) {
	env, _ := fixture(t)
	path := filepath.Join(env.Vault.Dir, "settings.json")
	put(t, path, "{broken")
	a := newTestApp(t, env, "en", 100, 30)
	press(a, "6")
	settle(a, press(a, "right"))
	if a.Settings.Theme != "nyxon-dark" || a.theme != "nyxon-dark" {
		t.Errorf("the theme should change for this run, got %q", a.Settings.Theme)
	}
	if read(t, path) != "{broken" || a.toast.kind != toastWarn {
		t.Errorf("the file should be left alone with a warning; toast %q", a.toast.text)
	}
}
