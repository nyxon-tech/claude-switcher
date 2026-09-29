package ui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A switch while Desktop runs, through the real program loop: the runner asks Desktop to quit,
// waits until it has, switches, reopens Desktop and toasts.
func TestRunnerSwitchesAroundDesktop(t *testing.T) {
	env, desk := fixture(t)
	desk.running, desk.quitErr = true, platform.ErrManualQuit
	tm := teatest.NewTestModel(t, newApp(env, Options{Version: "3.0.0", Mode: rtl.App, Theme: "dark"}),
		teatest.WithInitialTermSize(100, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte("Desktop running")) },
		teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyPressMsg{Code: tea.KeyLeft}) // from work to personal
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte("Switched to personal")) },
		teatest.WithDuration(5*time.Second))
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	if quits, forces, launches := desk.count(); quits != 1 || forces != 0 || launches != 1 {
		t.Errorf("quits %d, forces %d, launches %d; want 1, 0, 1", quits, forces, launches)
	}
	if env.Vault.Current() != "personal" {
		t.Errorf("profile in use is %q, want personal", env.Vault.Current())
	}
	if got := read(t, filepath.Join(env.Install.DataDir, "config.json")); got != login(personID) {
		t.Errorf("Desktop's login is %s, want personal's", got)
	}
}

// While it waits on Windows the runner shows the tray instruction, offers to force Desktop
// closed after asking, and esc cancels without changing anything.
func TestRunnerWaitsForTheTray(t *testing.T) {
	env, desk := fixture(t)
	desk.running = true
	a := newTestApp(t, env, 100, 30)
	a.screens[0].(*accountsScreen).focus = 1 // personal
	press(a, "enter")
	if a.runner == nil {
		t.Fatal("enter on another account should start the runner")
	}
	a.Update(a.runner.try()()) // the switch refuses: Desktop is open
	if a.runner.phase != waiting {
		t.Fatalf("runner phase %d, want waiting", a.runner.phase)
	}
	a.Update(quitMsg{platform.ErrManualQuit})
	checkFrame(t, a)
	text := strings.Join(screen(a), "\n")
	for _, want := range []string{i18n.T("ui.step.wait"), "right-click the Claude", "force close", i18n.T("ui.step.open")} {
		if !strings.Contains(text, want) {
			t.Errorf("waiting runner lacks %q:\n%s", want, text)
		}
	}

	press(a, "f")
	if a.dialog == nil || a.dialog.focus != 0 {
		t.Fatalf("f should ask first, with Cancel focused; dialog %+v", a.dialog)
	}
	a.Update(press(a, "y")())
	if _, forces, _ := desk.count(); forces != 1 {
		t.Errorf("forces %d after agreeing, want 1", forces)
	}

	press(a, "esc")
	a.Update(closedMsg{context.Canceled}) // what ops.WaitClosed returns once cancelled
	if a.runner != nil || a.toast.text != i18n.T("ui.run.cancelled") {
		t.Errorf("esc should cancel: runner %v, toast %q", a.runner, a.toast.text)
	}
	if env.Vault.Current() != "work" {
		t.Errorf("a cancelled switch changed the profile to %q", env.Vault.Current())
	}
}

// Saving over a profile of another account asks first, then replaces it.
func TestSaveAsksBeforeReplacing(t *testing.T) {
	const other = "9e9e9e9e-4444-4444-8444-444444444444"
	env, desk := fixture(t)
	put(t, filepath.Join(env.Install.DataDir, "config.json"), login(other))
	a := newTestApp(t, env, 100, 30)
	s := a.screens[0].(*accountsScreen)

	s.saveAs(a.Ctx, "work", false)
	a.Update(a.runner.try()())
	if a.runner != nil || a.dialog == nil || a.dialog.title != i18n.T("ui.save.replace.title", "name", "work") {
		t.Fatalf("other-account should turn into a question; runner %v, dialog %+v", a.runner, a.dialog)
	}
	press(a, "y")
	a.Update(a.runner.try()())
	if a.runner != nil || a.toast.text != i18n.T("ui.save.done", "name", "work") {
		t.Fatalf("replacing should finish with a toast; runner %v, toast %q", a.runner, a.toast.text)
	}
	if got := read(t, filepath.Join(env.Vault.Dir, "work", "_account")); got != other {
		t.Errorf("work's account is %s, want the signed-in %s", got, other)
	}
	if _, _, launches := desk.count(); launches != 0 {
		t.Errorf("Desktop was closed before, so it should stay closed; launches %d", launches)
	}
}

// Boxes stay inside the smallest window the app supports.
func TestBoxesFitSmallWindows(t *testing.T) {
	env, desk := fixture(t)
	desk.running = true
	a := newTestApp(t, env, 60, 18)
	a.screens[0].(*accountsScreen).focus = 1
	press(a, "d")
	checkFrame(t, a)
	press(a, "esc", "enter")
	a.Update(a.runner.try()())
	a.Update(quitMsg{platform.ErrManualQuit})
	checkFrame(t, a)
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, i18n.T("ui.key.force")) {
		t.Errorf("the runner's keys should stay visible:\n%s", text)
	}
}

// The force-close question goes away once Desktop closes on its own, and answering a stale one
// never ends Desktop (by then it may be open again after the change).
func TestForceQuestionExpires(t *testing.T) {
	env, desk := fixture(t)
	desk.running = true
	a := newTestApp(t, env, 100, 30)
	press(a, "left", "enter")
	a.Update(a.runner.try()())
	a.Update(quitMsg{platform.ErrManualQuit})
	press(a, "f")
	ask := a.dialog
	desk.running = false // the user quit it from the tray meanwhile
	a.Update(closedMsg{nil})
	if a.dialog != nil {
		t.Error("the force-close question should close with Desktop")
	}
	if cmd := ask.ok(ask); cmd != nil {
		t.Error("a stale force-close answer should do nothing")
	}
	if _, forces, _ := desk.count(); forces != 0 {
		t.Errorf("forces %d, want 0", forces)
	}
}

// ctrl+c waits while Claude's files are being written, and quits otherwise.
func TestCtrlCWaitsForTheWrite(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, 100, 30)
	press(a, "left", "enter")
	press(a, "ctrl+c")
	if a.toast.text != i18n.T("ui.run.busy") {
		t.Fatalf("ctrl+c during a write should ask to wait; toast %q", a.toast.text)
	}
	a.Update(a.runner.try()())
	if _, ok := press(a, "ctrl+c")().(tea.QuitMsg); !ok {
		t.Error("ctrl+c should quit once the change is written")
	}
}

// When Desktop's state cannot be read while waiting, the runner says so instead of "cancelled".
func TestWaitErrorIsShown(t *testing.T) {
	env, desk := fixture(t)
	desk.running = true
	a := newTestApp(t, env, 100, 30)
	press(a, "left", "enter")
	a.Update(a.runner.try()())
	a.Update(closedMsg{errors.New("cannot list processes")})
	if a.runner != nil || a.toast.kind != toastFail || a.toast.text != "cannot list processes" {
		t.Errorf("runner %v, toast %d %q", a.runner, a.toast.kind, a.toast.text)
	}
}
