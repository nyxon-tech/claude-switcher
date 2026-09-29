package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
)

// m on the account in use shows the plan's counts, then copies the other account's chats in.
func TestMergeBringsChats(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	a.Update(press(a, "m")()) // work has one chat list, so the plan is worked out at once
	if a.dialog == nil || !strings.Contains(a.dialog.body, i18n.T("ui.merge.counts", "new", 2, "updated", 0, "newer", 0)) {
		t.Fatalf("m should show the plan's counts first; dialog %+v", a.dialog)
	}
	press(a, "y")
	a.Update(a.runner.try()())
	if a.runner != nil || !strings.Contains(a.toast.text, "2") {
		t.Fatalf("the merge should finish with a toast; runner %v, toast %q", a.runner, a.toast.text)
	}
	cards, err := os.ReadDir(filepath.Join(env.Install.DataDir, "claude-code-sessions", workID, orgID))
	if err != nil || len(cards) != 5 {
		t.Errorf("work's list holds %d cards (%v), want its 3 plus personal's 2", len(cards), err)
	}
}

// A plan that arrives after the user started something else is dropped, never opened over it.
func TestLateMergePlanIsDropped(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	plan := press(a, "m")
	press(a, "left", "enter") // switch to personal meanwhile
	a.Update(plan())
	if a.dialog != nil {
		t.Errorf("a late plan opened %q over the switch", a.dialog.title)
	}
}

// Rename keeps the focus on the renamed card although it moves; remove asks first, with Cancel
// focused.
func TestRenameAndRemove(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	s := a.screens[0].(*accountsScreen)
	s.focus = 0 // client
	press(a, "r")
	a.dialog.input.SetValue("zeta")
	a.Update(press(a, "enter")())
	a.Update(a.Refresh()())
	if _, ok := env.Vault.Find("zeta"); !ok || a.toast.text != i18n.T("ui.rename.done", "old", "client", "name", "zeta") {
		t.Fatalf("client should be renamed to zeta; toast %q", a.toast.text)
	}
	if ps := s.profiles(a.Ctx); ps[s.focus].Name != "zeta" {
		t.Errorf("focus is on %q, want the renamed zeta", ps[s.focus].Name)
	}

	s.focus = 0 // personal
	if press(a, "d", "enter") != nil || a.dialog != nil {
		t.Fatal("enter on the remove question should take the focused Cancel")
	}
	a.Update(press(a, "d", "y")())
	if _, ok := env.Vault.Find("personal"); ok {
		t.Error("y should remove personal")
	}
}

// A box taller than a short window drops its blank lines so its buttons stay on screen.
func TestTallDialogFits(t *testing.T) {
	for _, lang := range []string{"en", "fa"} {
		t.Run(lang, func(t *testing.T) {
			env, _ := fixture(t)
			a := newTestApp(t, env, lang, 60, 18)
			press(a, "n", "!", "x", "enter")
			if a.dialog == nil || a.dialog.problem == "" {
				t.Fatal("a bad name should keep the dialog open with the problem")
			}
			checkFrame(t, a)
			box := a.dialog.view(a.Ctx, a.Width, a.Height-2)
			last := ansi.Strip(box[strings.LastIndex(box, "\n")+1:])
			if h := lipgloss.Height(box); h > a.Height-2 || !strings.Contains(last, "╰") {
				t.Errorf("the box is %d rows for %d, last row %q:\n%s", h, a.Height-2, last, ansi.Strip(box))
			}
			if text := strings.Join(screen(a), "\n"); !strings.Contains(text, a.L().Text(i18n.T("action.add"))) {
				t.Errorf("the Add button should stay on screen:\n%s", text)
			}
		})
	}
}

// A slow status read that returns after a newer one never replaces it.
func TestOlderStatusIsIgnored(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	older := a.Refresh()()
	if err := env.Vault.SetCurrent("personal"); err != nil {
		t.Fatal(err)
	}
	a.Update(a.Refresh()())
	a.Update(older)
	if a.Status.Current != "personal" {
		t.Errorf("status shows %q after an older read, want personal", a.Status.Current)
	}
}
