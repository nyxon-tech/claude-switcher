package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

func TestChatsSearch(t *testing.T) {
	a, s, _ := chatsFixture(t, 100, 30)
	for _, tc := range []struct {
		name, query string
		want        []string
	}{
		{"persian without zwnj", "صفحهی", []string{persianTitle}},
		{"persian with zwnj", "صفحه‌ی ورود", []string{persianTitle}},
		{"arabic yeh", "بازنويسي", []string{persianTitle}},
		{"arabic kaf", "كلاس", []string{notesTitle}},
		{"persian folder", "درسها", []string{notesTitle}},
		{"fuzzy", "fxlgn", []string{"Fix the login redirect"}},
		{"list label in All", "work", []string{"Fix the login redirect", persianTitle, "Release notes"}},
		{"one letter is a substring", "x", []string{"Fix the login redirect", "Old experiment"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for s.typing { // clear, then leave the field
				chatKeys(a, "esc")
			}
			chatType(a, tc.query)
			if !s.typing {
				t.Fatal("typing should focus the search field")
			}
			if got := chatTitles(s); strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("%q shows %q, want %q", tc.query, got, tc.want)
			}
			checkFrame(t, a)
		})
	}
	count := i18n.T("chats.search.count", "shown", 2, "total", 7)
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, count) {
		t.Errorf("the search row should say %q:\n%s", count, text)
	}
	chatKeys(a, "esc")
	a.Update(tea.PasteMsg{Content: "كلاس"})
	if got := chatTitles(s); len(got) != 1 || got[0] != notesTitle {
		t.Errorf("a paste should search too, got %q", got)
	}

	// q and digits are typed while the field has focus; esc clears, then leaves.
	chatKeys(a, "esc")
	chatType(a, "q2")
	if s.input.Value() != "q2" || a.tab != 1 {
		t.Fatalf("q and 2 should reach the field, got %q on tab %d", s.input.Value(), a.tab)
	}
	if chatKeys(a, "esc"); s.input.Value() != "" || !s.typing {
		t.Fatalf("esc should clear the field first, got %q typing %v", s.input.Value(), s.typing)
	}
	if chatKeys(a, "esc"); s.typing || a.tab != 1 {
		t.Fatalf("a second esc should leave the field, typing %v tab %d", s.typing, a.tab)
	}
	if chatKeys(a, "esc"); a.tab != 0 {
		t.Errorf("esc with nothing to clear should go back to Accounts, tab %d", a.tab)
	}
}

func TestChatsSelection(t *testing.T) {
	a, s, _ := chatsFixture(t, 100, 30)
	chatKeys(a, "space", "down", "space")
	if len(s.selected) != 2 {
		t.Fatalf("space should select two rows, got %d", len(s.selected))
	}
	text := strings.Join(screen(a), "\n")
	hint := i18n.T("chats.key.selected", "n", 2)
	if !strings.Contains(text, "☑") || !strings.Contains(text, "☐") || !strings.Contains(text, hint) {
		t.Errorf("selected rows get ☑, the others ☐, and the footer says %q:\n%s", hint, text)
	}
	checkFrame(t, a)
	if chatKeys(a, "space"); len(s.selected) != 1 {
		t.Errorf("space again should unselect, got %d", len(s.selected))
	}
	if chatKeys(a, "ctrl+a"); len(s.selected) != len(s.all) {
		t.Errorf("ctrl+a should select every row shown, got %d of %d", len(s.selected), len(s.all))
	}
	if chatKeys(a, "ctrl+a"); len(s.selected) != 0 {
		t.Errorf("ctrl+a with all selected should clear them, got %d", len(s.selected))
	}
	chatKeys(a, "space", "tab")
	if len(s.selected) != 0 {
		t.Error("switching chips should clear the selection")
	}
	chatKeys(a, "space", "esc")
	if len(s.selected) != 0 || a.tab != 1 {
		t.Errorf("esc should clear the selection and stay, got %d on tab %d", len(s.selected), a.tab)
	}
}

func TestChatsChips(t *testing.T) {
	a, s, _ := chatsFixture(t, 120, 40)
	var got []int
	for range 4 {
		chatKeys(a, "tab")
		got = append(got, len(s.rows()))
	}
	if fmt.Sprint(got) != "[3 4 1 7]" {
		t.Errorf("tab should go work, personal, Lost, All; rows %v", got)
	}
	if chatKeys(a, "shift+tab"); s.chip != chipLost {
		t.Errorf("shift+tab from All should open Lost, got %q", s.chip)
	}
	a.View()
	for i, k := range s.geo.chipKeys {
		if k == s.lists[1].Dir { // personal
			sp := s.geo.chips[i]
			a.Update(tea.MouseClickMsg{X: (sp.x0 + sp.x1) / 2, Y: 2 + chatsChipsY, Button: tea.MouseLeft})
		}
	}
	if s.chip != s.lists[1].Dir || chatTitles(s)[0] != "Trip plan" {
		t.Errorf("clicking the personal chip should open it, got %q %q", s.chip, chatTitles(s))
	}
}

func TestChatsClickRows(t *testing.T) {
	a, s, _ := chatsFixture(t, 120, 40)
	a.View()
	x := s.geo.listW / 2
	a.Update(tea.MouseClickMsg{X: x, Y: 2 + chatsListY + 2, Button: tea.MouseLeft})
	if s.cursor != 2 {
		t.Fatalf("clicking the third row moved the cursor to %d", s.cursor)
	}
	if a.Update(tea.MouseClickMsg{X: x, Y: 2 + chatsListY + 2, Button: tea.MouseLeft}); !s.full {
		t.Error("clicking the row under the cursor should open it")
	}
}

// The preview is read once the cursor has rested for chatsDebounce, never for a row passed by.
func TestChatsPreviewDebounce(t *testing.T) {
	a, s, _ := chatsFixture(t, 120, 40)
	first := s.previewSeq
	tick := chatKeys(a, "down")
	chatKeys(a, "down")
	if s.previewSeq != first+2 {
		t.Fatalf("each move should start a new wait, seq %d -> %d", first, s.previewSeq)
	}
	start := time.Now()
	msg := tick()
	if waited := time.Since(start); waited < chatsDebounce-10*time.Millisecond {
		t.Errorf("the preview waited %v, want %v", waited, chatsDebounce)
	}
	if _, ok := msg.(chatsTickMsg); !ok {
		t.Fatalf("the wait ends with %T", msg)
	}
	if a.Update(msg); s.reading != "" {
		t.Fatal("a row passed by should not be read")
	}
	chatsRun(a, s.readPreview(a.Ctx))
	r, _ := s.current()
	p, ok := s.previews[previewKey(r)]
	if !ok || p.err != nil || p.meta.Prompts != 1 {
		t.Fatalf("the rested row's preview should be read: %+v", p)
	}
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, i18n.T("chats.preview.prompts", "n", 1)) {
		t.Errorf("the preview should show the chat's counts:\n%s", text)
	}
	if chatKeys(a, "up"); chatKeys(a, "down") != nil {
		t.Error("a preview read before shows at once, with no wait")
	}
}

// A huge reply costs a frame no more than a short one: only what the preview shows is laid out.
// Escape codes pasted into a prompt never reach the terminal.
func TestChatsPreviewLongAndRawText(t *testing.T) {
	a, s, env := chatsFixture(t, 120, 40)
	chatTranscript(t, env, chatID(workID, 2), "", now.Add(-2*time.Hour),
		"\x1b[2JWrite the release notes for 3.0.", strings.Repeat("A line of the build log. ", 8000))
	chatKeys(a, "down", "down", "down", "down") // Release notes
	chatsRun(a, s.readPreview(a.Ctx))
	start := time.Now()
	for range 20 {
		a.View()
	}
	if frame := time.Since(start) / 20; frame > 16*time.Millisecond {
		t.Errorf("a 200 KB reply makes a frame take %v", frame)
	}
	if strings.Contains(a.View().Content, "\x1b[2J") {
		t.Error("an escape code from a prompt reached the terminal")
	}
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, "[2JWrite the release notes") || !strings.Contains(text, "…") {
		t.Errorf("the prompt shows without its escape, and the reply is cut short:\n%s", text)
	}
}

func TestChatsFullPreview(t *testing.T) {
	a, s, _ := chatsFixture(t, 80, 24)
	chatsRun(a, chatKeys(a, "enter"))
	if !s.full {
		t.Fatal("enter should open the preview full-screen")
	}
	checkFrame(t, a)
	text := strings.Join(screen(a), "\n")
	if !strings.Contains(text, "token exchange.") || !strings.Contains(text, i18n.T("chats.key.scroll")) {
		t.Errorf("the full preview shows every line of the messages:\n%s", text)
	}
	chatKeys(a, "end")
	if !s.pager.AtBottom() {
		t.Error("end should scroll to the bottom")
	}
	if chatKeys(a, "esc"); s.full || a.tab != 1 {
		t.Errorf("esc should close the full preview only, full %v tab %d", s.full, a.tab)
	}
}

// c copies the selected chats: pick the list, see the plan, confirm. Claude Desktop is open, so
// the runner closes it around the write and opens it again; the cards are then on disk.
func TestChatsCopy(t *testing.T) {
	a, s, env := chatsFixture(t, 100, 30)
	desk := env.Desktop.(*fakeDesktop)
	desk.running = true
	chatKeys(a, "tab") // work
	chatKeys(a, "space", "down", "down", "space")
	chatKeys(a, "c")
	if a.dialog == nil || len(a.dialog.choices) != 1 || a.dialog.cursor != 0 {
		t.Fatalf("c should offer personal, pre-selected; dialog %+v", a.dialog)
	}
	chatsRun(a, chatKeys(a, "enter"))
	counts := i18n.T("chats.plan.counts", "new", 2, "updated", 0, "newer", 0)
	if a.dialog == nil || !strings.Contains(a.dialog.body, counts) {
		t.Fatalf("the plan should show %q; dialog %+v", counts, a.dialog)
	}
	checkFrame(t, a)

	chatKeys(a, "y")
	a.Update(a.runner.try()()) // ops refuses while Desktop runs, before writing anything
	if a.runner == nil || a.runner.phase != waiting {
		t.Fatalf("the copy should wait for Desktop to close; runner %+v", a.runner)
	}
	checkFrame(t, a)
	a.Update(quitMsg{desk.Quit()})
	a.Update(closedMsg{env.WaitClosed(context.Background())})
	a.Update(a.runner.try()())
	a.Update(launchedMsg{desk.Launch()})
	if a.runner != nil || !strings.Contains(a.toast.text, "Copied 2 chats") || !strings.Contains(a.toast.text, i18n.T("chats.undo_hint")) {
		t.Fatalf("the copy should finish with a toast; runner %v, toast %q", a.runner, a.toast.text)
	}
	if quits, forces, launches := desk.count(); quits != 1 || forces != 0 || launches != 1 {
		t.Errorf("quits %d, forces %d, launches %d; want 1, 0, 1", quits, forces, launches)
	}
	cards, err := os.ReadDir(filepath.Join(env.Install.DataDir, "claude-code-sessions", personID, orgID))
	if err != nil || len(cards) != 6 {
		t.Fatalf("personal holds %d cards (%v), want its 4 and the 2 copied", len(cards), err)
	}

	s.Update(a.Ctx, statusMsg{})
	if !s.loading || s.stale || len(s.selected) != 0 {
		t.Errorf("once the change is done the lists are read again, with nothing selected")
	}
	chatsRun(a, s.load(a.Ctx))
	chatKeys(a, "tab") // personal
	if got := strings.Join(chatTitles(s), "|"); !strings.Contains(got, "Fix the login redirect") || !strings.Contains(got, "Release notes") {
		t.Errorf("the personal chip should show the copied chats: %s", got)
	}
}

func TestChatsMoveNeedsOneList(t *testing.T) {
	a, _, _ := chatsFixture(t, 100, 30)
	chatKeys(a, "space", "down", "space") // Fix the login redirect (work), Trip plan (personal)
	chatKeys(a, "m")
	if a.dialog == nil || a.dialog.title != i18n.T("chats.mixed.title") {
		t.Fatalf("a selection from two lists should be refused; dialog %+v", a.dialog)
	}
	chatKeys(a, "esc", "esc")
	chatKeys(a, "m") // the current row alone: Trip plan
	if a.dialog == nil || len(a.dialog.choices) != 1 {
		t.Fatalf("m on one row should offer the other list; dialog %+v", a.dialog)
	}
	chatsRun(a, chatKeys(a, "enter"))
	if a.dialog == nil || !strings.Contains(a.dialog.body, i18n.T("chats.move.body", "from", "personal", "to", "work")) {
		t.Errorf("a move says the chats leave the source; dialog %+v", a.dialog)
	}
}

// In Lost, r recovers the chat into the signed-in list.
func TestChatsRescue(t *testing.T) {
	a, s, env := chatsFixture(t, 120, 40)
	chatKeys(a, "shift+tab")
	if s.chip != chipLost || len(s.shown) != 1 || s.hidden != 1 {
		t.Fatalf("Lost should hold the pricing page once, its older part left out: %d rows, %d hidden", len(s.shown), s.hidden)
	}
	checkFrame(t, a)
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, i18n.T("chats.lost.hidden", "n", 1)) {
		t.Errorf("Lost should say an older part is left out:\n%s", text)
	}
	chatKeys(a, "r")
	if a.dialog == nil || s.lists[a.dialog.cursor].Account != workID {
		t.Fatalf("r should offer the lists with the signed-in one pre-selected; dialog %+v", a.dialog)
	}
	chatKeys(a, "enter", "y")
	a.Update(a.runner.try()())
	if !strings.Contains(a.toast.text, "Recovered 1 chat into work") {
		t.Fatalf("toast %q", a.toast.text)
	}
	lists, _ := env.Lists()
	chats, err := env.Chats(lists[0])
	if err != nil || len(chats) != 4 || chats[3].Session != chatLostID {
		t.Fatalf("work should hold the recovered chat: %d chats (%v)", len(chats), err)
	}
	chatsRun(a, s.loadLost(a.Ctx))
	chatsRun(a, s.load(a.Ctx))
	checkFrame(t, a)
	if text := strings.Join(screen(a), "\n"); s.chip != chipLost || !strings.Contains(text, i18n.T("chats.lost.empty.title")) {
		t.Errorf("Lost stays open and says nothing is lost:\n%s", text)
	}
}

func TestChatsExport(t *testing.T) {
	a, s, _ := chatsFixture(t, 100, 30)
	s.downloads = t.TempDir()
	for _, want := range []string{"Fix the login redirect.html", "Fix the login redirect-2.html"} {
		chatKeys(a, "e")
		if a.dialog == nil || len(a.dialog.choices) != 2 {
			t.Fatalf("e should ask for the format; dialog %+v", a.dialog)
		}
		chatsRun(a, chatKeys(a, "enter"))
		path := filepath.Join(s.downloads, want)
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "token exchange") {
			t.Fatalf("%s: %v", want, err)
		}
		if a.toast.text != i18n.T("chats.export.done", "path", path) {
			t.Errorf("toast %q", a.toast.text)
		}
	}
	chatKeys(a, "down", "down", "down", "e", "down") // Budget sheet, as Markdown: it has no history left
	chatsRun(a, chatKeys(a, "enter"))
	if a.toast.kind != toastFail {
		t.Errorf("exporting a chat without history should fail, toast %q", a.toast.text)
	}
	if files, _ := os.ReadDir(s.downloads); len(files) != 2 {
		t.Errorf("a failed export should leave no file, got %d files", len(files))
	}
}

func TestExportName(t *testing.T) {
	for _, tc := range []struct{ title, want string }{
		{"Fix the login redirect", "Fix the login redirect"},
		{`a/b\c:d*e?f"g<h>i|j`, "a b c d e f g h i j"},
		{"trailing dots...  ", "trailing dots"},
		{"CON", "CON_"},
		{"nul.notes", "nul_.notes"},
		{"com1", "com1_"},
		{"Console", "Console"},
		{"", "chat"},
		{"...", "chat"},
		{persianTitle, persianTitle},
		{"line\nbreak\ttab", "line break tab"},
	} {
		if got := exportName(tc.title); got != tc.want {
			t.Errorf("exportName(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
	if got := exportName(strings.Repeat("x", 200)); len(got) != 80 {
		t.Errorf("a long title should be cut to 80 characters, got %d", len(got))
	}
}

func TestChatsEmpty(t *testing.T) {
	env, _ := fixture(t)
	if err := os.RemoveAll(filepath.Join(env.Install.DataDir, "claude-code-sessions")); err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, env, "en", 80, 24)
	chatsRun(a, press(a, "2"))
	checkFrame(t, a)
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, i18n.T("chats.empty.title")) {
		t.Errorf("no chats at all should say so:\n%s", text)
	}
	chatType(a, "zz")
	if text := strings.Join(screen(a), "\n"); !strings.Contains(text, i18n.T("chats.nomatch.body")) {
		t.Errorf("a search with no match should say so:\n%s", text)
	}
}

// With a thousand chats, drawing a frame, typing a letter and scrolling a row each take less
// than a frame at 60 Hz: only the rows shown are drawn, and the search text is folded once.
func TestChatsThousand(t *testing.T) {
	a, s, _ := chatsFixture(t, 120, 40)
	a.Update(thousandChats(s))
	if len(s.all) != 1000 {
		t.Fatalf("%d rows", len(s.all))
	}
	const runs = 50
	timed := func(step func(i int)) time.Duration {
		start := time.Now()
		for i := range runs {
			step(i)
		}
		return time.Since(start) / runs
	}
	frame := timed(func(int) { a.View() })
	typing := timed(func(i int) {
		chatKeys(a, string(rune('a'+i%26)))
		if i%3 == 2 {
			chatKeys(a, "esc")
		}
	})
	chatKeys(a, "esc", "esc") // clear the search and leave the field
	scroll := timed(func(int) { chatKeys(a, "down"); a.View() })
	t.Logf("a frame takes %v, a key in the search %v, a row of scrolling %v", frame, typing, scroll)
	if limit := 16 * time.Millisecond; frame > limit || typing > limit || scroll > limit {
		t.Errorf("too slow with 1000 chats: a frame %v, a key %v, scrolling %v", frame, typing, scroll)
	}

	chatKeys(a, "end")
	if text := strings.Join(screen(a), "\n"); s.cursor != 999 || !strings.Contains(text, " 999") {
		t.Errorf("end should show the last chat, cursor %d:\n%s", s.cursor, text)
	}
	if chatKeys(a, "home", "pgdown"); s.cursor != s.geo.rows {
		t.Errorf("pgdown from the top should move a page (%d rows), cursor %d", s.geo.rows, s.cursor)
	}
	checkFrame(t, a)
}

// thousandChats is the lists' load again with a thousand chats in the first list.
func thousandChats(s *chatsScreen) chatsLoadedMsg {
	msg := chatsLoadedMsg{gen: s.gen, lists: s.lists, chats: make([][]ops.Chat, len(s.lists))}
	words := []string{"Fix", "the", "login", "redirect", "بازنویسی", "صفحه‌ی", "ورود", "release", "notes", "پرداخت"}
	for i := range 1000 {
		title := fmt.Sprintf("%s %s %s %d", words[i%10], words[(i/10)%10], words[(i/100)%10], i)
		msg.chats[0] = append(msg.chats[0], ops.Chat{
			ID: fmt.Sprintf("local_%d", i), Title: title, Project: fmt.Sprintf("project-%d", i%17),
			Last: now.Add(-time.Duration(i) * time.Minute), HasHistory: i%9 != 0, Archived: i%23 == 0,
			List: s.lists[0], Path: fmt.Sprintf("/cards/%d.json", i),
		})
	}
	return msg
}
