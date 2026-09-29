package ops

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
)

func find(checks []Check, title string) (Check, bool) {
	for _, c := range checks {
		if c.Title == title {
			return c, true
		}
	}
	return Check{}, false
}

func TestDoctor(t *testing.T) {
	f, now := chatFixture(t)
	checks := f.Doctor()
	for _, c := range checks {
		if c.Level == Fail || c.Level == Warn {
			t.Errorf("a clean setup should pass: %+v", c)
		}
	}
	for title, level := range map[string]Level{
		i18n.T("ops.doctor.signed-in", "name", "work"):     OK,
		i18n.T("ops.doctor.list", "label", "work", "n", 2): OK,
		i18n.T("ops.doctor.no-lost"):                       OK,
		i18n.T("ops.doctor.desktop-kept"):                  OK,
		i18n.T("ops.doctor.terminal-cleanup", "n", 30):     Info,
	} {
		if c, ok := find(checks, title); !ok || c.Level != level {
			t.Errorf("missing %s check %q", level, title)
		}
	}

	f.transcript(lost(1), "Lost", "q", time.UnixMilli(now))
	f.card(work, "ghost", "10000000-0000-4000-8000-000000000099", "Gone", now, "")
	f.put(filepath.Join(filepath.Dir(f.Projects), "settings.json"), `{"cleanupPeriodDays":90,"desktopSessionCleanupPeriodDays":7}`)
	linked := filepath.Join(f.data, "claude-code-sessions", "44444444-4444-4444-8444-444444444444", org)
	must(t, os.MkdirAll(filepath.Dir(linked), 0o700))
	makeLink(t, f.listDir(pers), linked)
	f.put(filepath.Join(f.data, "config.json"), `{"lastKnownAccountUuid":"`+third+`"}`)

	checks = f.Doctor()
	for title, level := range map[string]Level{
		i18n.T("ops.doctor.lost", "n", 1):                                                        Warn,
		i18n.T("ops.doctor.ghosts", "n", 1, "label", "work"):                                     Info,
		i18n.T("ops.doctor.desktop-cleanup", "n", 7):                                             Warn,
		i18n.T("ops.doctor.terminal-cleanup", "n", 90):                                           Info,
		i18n.T("ops.doctor.unsaved"):                                                             Warn,
		i18n.T("ops.doctor.mismatch", "name", "work"):                                            Warn,
		i18n.T("ops.doctor.list", "label", i18n.T("ops.list.account", "id", "44444444"), "n", 1): Fail,
	} {
		if c, ok := find(checks, title); !ok || c.Level != level {
			t.Errorf("missing %s check %q in %+v", level, title, checks)
		}
	}
}

func TestWaitClosed(t *testing.T) {
	oldPoll, oldGrace := pollEvery, grace
	pollEvery, grace = time.Millisecond, time.Millisecond
	t.Cleanup(func() { pollEvery, grace = oldPoll, oldGrace })
	f := newFixture(t)

	must(t, f.WaitClosed(context.Background()))
	if f.desk.polls != 1 {
		t.Errorf("a closed Desktop should be seen at once, polled %d times", f.desk.polls)
	}
	f.desk.running, f.desk.closeAfter, f.desk.polls = true, 3, 0
	must(t, f.WaitClosed(context.Background()))
	if f.desk.polls != 4 {
		t.Errorf("polled %d times, want 4", f.desk.polls)
	}
	f.desk.running, f.desk.closeAfter = true, 0
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := f.WaitClosed(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want the context's error", err)
	}
}

func TestReadRefusals(t *testing.T) {
	f := newFixture(t)
	_, _, err := f.Preview(Chat{Title: "no transcript"})
	wantCode(t, err, NoHistory)
	wantCode(t, f.Export(Chat{Transcript: "x.jsonl"}, "pdf", &bytes.Buffer{}), BadFormat)
}
