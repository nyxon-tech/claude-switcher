package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

// firstRun is the fixture before the welcome was seen, with the signed-in account (work) not
// saved yet when unsaved is set.
func firstRun(t *testing.T, unsaved bool) (*ops.Env, *fakeDesktop) {
	t.Helper()
	env, desk := fixture(t)
	if err := os.Remove(filepath.Join(env.Vault.Dir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if unsaved {
		if err := os.RemoveAll(filepath.Join(env.Vault.Dir, "work")); err != nil {
			t.Fatal(err)
		}
	}
	return env, desk
}

// welcomed is the app showing the welcome's first step, the brand.
func welcomed(t *testing.T, env *ops.Env, width, height int) *app {
	t.Helper()
	a := newTestApp(t, env, width, height)
	a.Welcome()
	return a
}

func TestOnboardingFrames(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}, {120, 40}} {
		for _, step := range []string{"hello", "save"} {
			t.Run(fmt.Sprintf("en_%dx%d_%s", size[0], size[1], step), func(t *testing.T) {
				env, _ := firstRun(t, true)
				a := welcomed(t, env, size[0], size[1])
				want := []string{"C L A U D E   S W I T C H E R", i18n.T("app.by") + " nyxon", i18n.T("app.tagline"), i18n.T("welcome.key.skip")}
				if step == "save" {
					press(a, "enter")
					want = []string{i18n.T("welcome.save.title"), "work", i18n.T("welcome.key.later")}
				}
				checkFrame(t, a)
				text := strings.Join(screen(a), "\n")
				for _, w := range want {
					if !strings.Contains(text, w) {
						t.Errorf("the %s step lacks %q", step, w)
					}
				}
				golden.RequireEqual(t, text)
			})
		}
	}
}

// The smallest window the app draws in still holds Settings and every step of the welcome.
func TestSmallestWindowFits(t *testing.T) {
	env, _ := firstRun(t, true)
	a := welcomed(t, env, 60, 18)
	checkFrame(t, a)
	press(a, "enter")
	checkFrame(t, a)
	press(a, "esc", "6")
	if a.onboard != nil || a.tab != 5 {
		t.Fatalf("esc should end the welcome and 6 open Settings; tab %d", a.tab)
	}
	checkFrame(t, a)
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, i18n.T("settings.welcome")) {
		t.Errorf("every setting should show in a 60x18 window:\n%s", text)
	}
}

// The first run: the brand, then enter asks for a name for the signed-in account, and saving it
// (the way Accounts saves) ends the welcome on Accounts, remembered as seen.
func TestOnboardingFlow(t *testing.T) {
	env, _ := firstRun(t, true)
	a := newTestApp(t, env, 100, 30)
	settle(a, a.Init())
	o := a.onboard
	if o == nil {
		t.Fatal("the first run should open the welcome")
	}
	press(a, "enter")
	if !o.saving || o.input.Value() != "work" {
		t.Fatalf("the unsaved signed-in account should be asked for, suggesting work; got %q", o.input.Value())
	}
	settle(a, press(a, "enter"))
	if a.onboard != nil || a.tab != 0 || a.runner != nil {
		t.Fatalf("saving should end the welcome on Accounts; tab %d, runner %v", a.tab, a.runner)
	}
	if p, ok := env.Vault.Find("work"); !ok || p.Account != workID {
		t.Errorf("work should be saved from the signed-in account, got %+v", p)
	}
	if a.toast.text != i18n.T("ui.save.done", "name", "work") || savedSettings(t, a)["onboarded"] != true {
		t.Errorf("toast %q; the welcome should be remembered as seen", a.toast.text)
	}
}

// Naming the account is skipped when there is nothing to save, and esc (skip, later) ends the
// welcome at once, saving nothing.
func TestOnboardingSkips(t *testing.T) {
	for _, tc := range []struct {
		name               string
		unsaved, signedOut bool
		keys               []string
		naming             bool // the welcome asks for a name after keys; otherwise it has ended
	}{
		{"account saved already", false, false, []string{"enter"}, false},
		{"desktop signed out", true, true, []string{"enter"}, false},
		{"account to save", true, false, []string{"enter"}, true},
		{"esc skips", true, false, []string{"esc"}, false},
		{"later", true, false, []string{"enter", "esc"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := firstRun(t, tc.unsaved)
			if tc.signedOut {
				if err := os.Remove(filepath.Join(env.Install.DataDir, "config.json")); err != nil {
					t.Fatal(err)
				}
			}
			a := welcomed(t, env, 100, 30)
			for _, k := range tc.keys {
				settle(a, press(a, k))
			}
			switch {
			case tc.naming:
				if a.onboard == nil || !a.onboard.saving {
					t.Fatal("the welcome should ask for a name")
				}
				return
			case a.onboard != nil:
				t.Fatalf("the welcome should have ended; naming %v", a.onboard.saving)
			case !a.Settings.Onboarded || savedSettings(t, a)["onboarded"] != true:
				t.Error("the end of the welcome should be remembered")
			case a.tab != 0 || a.toast.text != i18n.T("welcome.done"):
				t.Errorf("the welcome should end on Accounts with a toast; tab %d, toast %q", a.tab, a.toast.text)
			}
			if _, ok := env.Vault.Find("work"); ok == tc.unsaved {
				t.Error("skipping must not save the account")
			}
		})
	}
}

// Enter before the state was read waits for it, and says so.
func TestOnboardingWaitsForTheState(t *testing.T) {
	env, _ := firstRun(t, true)
	a := newApp(env, Options{Version: "3.0.0", Mode: rtl.App, Theme: "dark"})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a.Welcome()
	press(a, "enter")
	if o := a.onboard; !o.waiting || o.saving {
		t.Fatal("enter should wait for the state")
	}
	if !strings.Contains(strings.Join(screen(a), "\n"), i18n.T("state.loading")) {
		t.Error("the welcome should say the state is loading")
	}
	a.Update(a.Refresh()())
	if o := a.onboard; o.waiting || !o.saving {
		t.Error("the state should move the welcome on to naming the account")
	}
}
