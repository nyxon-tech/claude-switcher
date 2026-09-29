package ui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

const (
	chatLostID   = "9f000000-0000-4000-8000-000000000001"
	persianTitle = "بازنویسی صفحه‌ی ورود" // a work chat of the shared fixture
	notesTitle   = "یادداشت‌های کلاس"     // a personal chat added here: ZWNJ, ک and a Persian folder
)

// chatID is the fixture's id (and transcript id) of an account's i-th chat.
func chatID(account string, i int) string {
	return fmt.Sprintf("%s-%04d-4000-8000-000000000000", account[:8], i)
}

// chatCard writes a card into an account's chat list.
func chatCard(t *testing.T, env *ops.Env, account, id, cwd, title string, last time.Time, archived bool) {
	t.Helper()
	card, err := json.Marshal(map[string]any{
		"sessionId": "local_" + id, "cliSessionId": id, "cwd": cwd, "title": title,
		"isArchived": archived, "lastActivityAt": last.UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(env.Install.DataDir, "claude-code-sessions", account, orgID, "local_"+id+".json"), string(card))
}

// chatTranscript writes a transcript of turns (user, assistant, user...) ending at end, a
// minute apart, in the fixture's projects folder.
func chatTranscript(t *testing.T, env *ops.Env, session, title string, end time.Time, turns ...string) {
	t.Helper()
	var b strings.Builder
	line := func(v map[string]any) {
		j, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(j)
		b.WriteByte('\n')
	}
	if title != "" {
		line(map[string]any{"type": "ai-title", "aiTitle": title, "sessionId": session})
	}
	for i, text := range turns {
		ts := end.Add(-time.Duration(len(turns)-1-i) * time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
		m := map[string]any{"cwd": "/work/proj", "sessionId": session, "uuid": fmt.Sprintf("%s-%d", session[:8], i), "timestamp": ts}
		if i%2 == 0 {
			m["type"], m["message"] = "user", map[string]any{"role": "user", "content": text}
		} else {
			m["type"], m["message"] = "assistant", map[string]any{"id": fmt.Sprintf("msg_%d", i), "model": "claude-opus-5",
				"role": "assistant", "content": []map[string]any{{"type": "text", "text": text}}}
		}
		line(m)
	}
	put(t, filepath.Join(env.Projects, "-work-proj", session+".jsonl"), b.String())
}

// chatsFixture is the app fixture plus transcripts for all but one of its chats (Budget sheet
// has none left), an archived chat, a Persian chat in a Persian folder with prompts in both
// languages, and a lost chat whose older part is left out, with the app on the Chats tab and
// everything read. All: Fix the login redirect, Trip plan, the Persian work chat, Budget sheet,
// Release notes, the Persian notes, Old experiment.
func chatsFixture(t *testing.T, width, height int) (*app, *chatsScreen, *ops.Env) {
	t.Helper()
	env, _ := fixture(t)
	chatTranscript(t, env, chatID(workID, 0), "", now.Add(-5*time.Minute),
		"The login page sends people back to the home page after OAuth. Keep them on the page they came from.",
		"The redirect target is dropped in the callback handler. I kept it in the state parameter and read it back after the token exchange.",
		"Add a test for it too.",
		"Added a test that signs in from /settings and checks the redirect lands there.")
	chatTranscript(t, env, chatID(workID, 1), "", now.Add(-time.Hour),
		"صفحه‌ی ورود را با فرم تازه بازنویسی کن و پیام‌های خطا را فارسی کن.",
		"صفحه را بازنویسی کردم و پیام‌های خطا حالا فارسی هستند.")
	chatTranscript(t, env, chatID(workID, 2), "", now.Add(-2*time.Hour), "Write the release notes for 3.0.", "Here is a draft.")
	chatTranscript(t, env, chatID(personID, 0), "", now.Add(-10*time.Minute), "Plan a three day trip to Isfahan.", "Day one: Naqsh-e Jahan Square.")
	archived := chatID(personID, 2)
	chatCard(t, env, personID, archived, "/work/notes", "Old experiment", now.Add(-5*24*time.Hour), true)
	chatTranscript(t, env, archived, "", now.Add(-5*24*time.Hour), "Try the old approach.", "Done.")
	notes := chatID(personID, 3)
	chatCard(t, env, personID, notes, "/work/درس‌ها", notesTitle, now.Add(-26*time.Hour), false)
	chatTranscript(t, env, notes, "", now.Add(-26*time.Hour),
		"خلاصه‌ی جلسه‌ی امروز را بنویس و در notes.md ذخیره کن؛ بخش «کارهای بعدی» را هم به‌روز کن.",
		"خلاصه را در notes.md نوشتم و سه کار تازه به «کارهای بعدی» افزودم.",
		"Add an English version under the Persian summary.",
		"Done: the English version follows the Persian one.")
	chatTranscript(t, env, chatLostID, "Draft the pricing page", now.Add(-3*time.Hour), "Draft the pricing page with three tiers.", "Here are three tiers.")
	chatTranscript(t, env, "9f000000-0000-4000-8000-000000000002", "Draft the pricing page", now.Add(-4*time.Hour), "Start the pricing page.", "Started.")

	a := newTestApp(t, env, width, height)
	s := a.screens[1].(*chatsScreen)
	chatsRun(a, press(a, "2"))
	if !s.loaded {
		t.Fatalf("the chats were not read: %v", s.err)
	}
	return a, s, env
}

// chatsRun runs cmd and hands its messages to the app. Commands those return are dropped, so
// no timer runs.
func chatsRun(a *app, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			chatsRun(a, c)
		}
	default:
		a.Update(msg)
	}
}

// chatKeys sends keys as the terminal does: named keys by code, anything else as typed text.
func chatKeys(a *app, keys ...string) tea.Cmd {
	codes := map[string]tea.Key{
		"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown}, "pgup": {Code: tea.KeyPgUp}, "pgdown": {Code: tea.KeyPgDown},
		"home": {Code: tea.KeyHome}, "end": {Code: tea.KeyEnd}, "tab": {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift}, "enter": {Code: tea.KeyEnter}, "esc": {Code: tea.KeyEscape},
		"space": {Code: tea.KeySpace, Text: " "}, "ctrl+a": {Code: 'a', Mod: tea.ModCtrl}, "backspace": {Code: tea.KeyBackspace},
	}
	var cmd tea.Cmd
	for _, k := range keys {
		msg, ok := codes[k]
		if !ok {
			msg = tea.Key{Code: []rune(k)[0], Text: k}
		}
		_, cmd = a.Update(tea.KeyPressMsg(msg))
	}
	return cmd
}

// chatType types text a letter at a time.
func chatType(a *app, text string) {
	for _, r := range text {
		chatKeys(a, string(r))
	}
}

// chatTitles are the titles of the rows the search shows, in order.
func chatTitles(s *chatsScreen) []string {
	var out []string
	rows := s.rows()
	for _, i := range s.shown {
		out = append(out, rows[i].Title)
	}
	return out
}

// Two chats selected and the cursor on the Persian notes, read: side by side in a wide window,
// the preview under the list in a narrow one.
func TestChatsFrames(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("en_%dx%d", size[0], size[1]), func(t *testing.T) {
			a, s, _ := chatsFixture(t, size[0], size[1])
			chatKeys(a, "space", "down", "down", "space", "down", "down", "down")
			chatsRun(a, s.readPreview(a.Ctx))
			checkFrame(t, a)
			text := strings.Join(screen(a), "\n")
			for _, want := range []string{
				"All 7", "work 3", "personal 4", "Lost 1", "Fix the login redirect", "☑", "☐",
				a.L().Data(notesTitle, 40), i18n.T("chats.preview.first"),
			} {
				if !strings.Contains(text, want) {
					t.Errorf("frame lacks %q", want)
				}
			}
			golden.RequireEqual(t, text)
		})
	}
}

// Persian user data reads right. Where the app lays it out (rtl.App) its letters are joined
// (presentation forms, no bare Arabic letters left) and each line is in visual order; where the
// terminal does (rtl.Terminal) it passes through as typed. Either way it keeps to its column.
func TestChatsPersianData(t *testing.T) {
	for _, tc := range []struct {
		mode          rtl.Mode
		title, folder string // as the terminal gets them
	}{
		{rtl.App, "ﺩﻭﺭﻭ ﯼﻪﺤﻔﺻ ﯽﺴﯾﻮﻧﺯﺎﺑ", "ﺎﻫﺱﺭﺩ"}, // joined, the first word at the right
		{rtl.Terminal, persianTitle, "درس‌ها"},
	} {
		for _, size := range [][2]int{{80, 24}, {120, 40}} {
			t.Run(fmt.Sprintf("%s_%dx%d", tc.mode, size[0], size[1]), func(t *testing.T) {
				a, s, _ := chatsFixture(t, size[0], size[1])
				a.Opts.Mode = tc.mode
				chatKeys(a, "down", "down") // the Persian work chat
				chatsRun(a, s.readPreview(a.Ctx))
				checkFrame(t, a)
				rows := screen(a)
				text := strings.Join(rows, "\n")
				if n := strings.Count(text, tc.title); n != 2 {
					t.Errorf("the title should show in its row and atop the preview, found %d times:\n%s", n, text)
				}
				if !strings.Contains(text, tc.folder) {
					t.Errorf("the Persian folder should show as %q:\n%s", tc.folder, text)
				}
				bare := strings.ContainsFunc(text, func(r rune) bool { return unicode.IsLetter(r) && r >= 0x0600 && r <= 0x06ff })
				if bare != (tc.mode == rtl.Terminal) {
					t.Errorf("bare Arabic letters: %v, want them only in terminal mode", bare)
				}
				if tc.mode == rtl.App {
					// A Persian prompt starts every line at the right: the preview's lines of it
					// end at the edge of the window.
					label := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, i18n.T("chats.preview.first")) })
					if label < 0 || strings.HasSuffix(rows[label+1], " ") {
						t.Errorf("the Persian prompt should end at the right edge:\n%s", text)
					}
				}
			})
		}
	}
}

// A title far longer than its column is cut to fit, in either rtl mode and at any width.
func TestChatsLongTitlesFit(t *testing.T) {
	a, s, _ := chatsFixture(t, 120, 40)
	long := strings.Repeat(persianTitle+" and some English ", 6)
	for i := range s.all {
		s.all[i].Title = long
	}
	for _, mode := range []rtl.Mode{rtl.App, rtl.Terminal} {
		a.Opts.Mode = mode
		for width := 20; width <= 120; width += 7 {
			for i, line := range append(s.listLines(a.Ctx, width, 12), s.previewPane(a.Ctx, width, 12)...) {
				if w := ansi.StringWidth(line); w != width {
					t.Errorf("%s at %d cells: line %d is %d cells: %q", mode, width, i, w, ansi.Strip(line))
				}
			}
		}
	}
}

// A paragraph keeps its direction on every line it wraps into: a line of a Persian prompt that
// starts with an English word still reads right to left, so the word sits at the right.
func TestChatWrapKeepsDirection(t *testing.T) {
	l := Layout{Mode: rtl.App}
	for _, tc := range []struct {
		text  string
		width int
		dir   rtl.Dir
		line2 string // the second line, as typed
	}{
		{"درست notes.md را", 11, rtl.RTL, "notes.md را"},
		{"Save it in یادداشت‌ها now", 13, rtl.LTR, "یادداشت‌ها now"},
	} {
		lines, dir := chatWrap(l, tc.text, tc.width, 0)
		if dir != tc.dir || len(lines) != 2 || lines[1] != l.Mode.Line(tc.line2, tc.dir) {
			t.Errorf("chatWrap(%q) = %q, %v; want the second line laid out as %v", tc.text, lines, dir, tc.dir)
		}
	}
}
