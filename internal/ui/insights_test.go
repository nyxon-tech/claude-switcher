package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// Tabs of the insight screens, as the keys that open them.
const (
	activityTab = "3"
	usageTab    = "4"
	doctorTab   = "5"
)

// activityFixture records five changes, as ops would: the newest minutes to hours old, one
// undone and into a list with a Persian name, the oldest older than a month.
func activityFixture(t *testing.T, env *ops.Env) {
	t.Helper()
	list := filepath.Join(env.Install.DataDir, "claude-code-sessions", workID, orgID)
	n := 0
	card := func() string {
		n++
		return filepath.Join(list, fmt.Sprintf("local_%08d-0000-4000-8000-000000000000.json", n))
	}
	for _, c := range []struct {
		action, summary  string
		ago              time.Duration
		created, removed int
		undone           bool
	}{
		{ops.Merge, i18n.T("ops.done.merge", "n", 2, "to", "work"), 20 * time.Minute, 2, 0, false},
		{ops.Copy, i18n.T("ops.done.copy", "n", 1, "from", "personal", "to", "خانه"), 3 * time.Hour, 1, 0, true},
		{ops.Move, i18n.T("ops.done.move", "n", 3, "from", "work", "to", "personal"), 50 * time.Hour, 3, 3, false},
		{ops.Rescue, i18n.T("ops.done.rescue", "n", 2, "to", "work"), 12 * 24 * time.Hour, 2, 0, false},
		{ops.Copy, i18n.T("ops.done.copy", "n", 12, "from", "personal", "to", "work"), 45 * 24 * time.Hour, 12, 0, false},
	} {
		j, err := env.Vault.NewJournal(c.action, now.Add(-c.ago))
		if err != nil {
			t.Fatal(err)
		}
		for range c.created {
			if err := j.Created(card()); err != nil {
				t.Fatal(err)
			}
		}
		for range c.removed {
			path := card()
			put(t, path, "{}")
			if err := j.Removed(path); err != nil {
				t.Fatal(err)
			}
		}
		if err := j.Save(c.summary); err != nil {
			t.Fatal(err)
		}
		if c.undone {
			put(t, filepath.Join(j.Dir, "undone"), now.Format(time.RFC3339))
		}
	}
}

// usageFixture writes 26 transcripts over three models and four folders: one a day for the last
// 25 days and one 44 days ago.
func usageFixture(t *testing.T, env *ops.Env) {
	t.Helper()
	models := []string{"claude-opus-5", "claude-opus-5", "claude-sonnet-4-5-20250929", "claude-sonnet-4-5-20250929", "claude-haiku-4-5"}
	folders := []string{"/work/api", "/work/web", "/home/me/notes", "/work/بازاریابی", "/work/api"}
	for i := range 26 {
		day := i
		if i == 25 {
			day = 44
		}
		at := now.Add(-time.Duration(day)*24*time.Hour - 2*time.Hour)
		id := fmt.Sprintf("%08d-0000-4000-8000-000000000000", i+1)
		var lines []string
		for m := range 1 + i%3 {
			line, err := json.Marshal(map[string]any{
				"type": "assistant", "timestamp": at.Add(time.Duration(m) * time.Minute).Format(time.RFC3339Nano),
				"cwd": folders[i%5], "sessionId": id,
				"message": map[string]any{"id": fmt.Sprintf("msg_%d_%d", i, m), "model": models[i%5], "role": "assistant",
					"usage": map[string]any{"input_tokens": 40, "output_tokens": 1500 + (i*37%11)*900,
						"cache_creation_input_tokens": 2500, "cache_read_input_tokens": 30000}},
			})
			if err != nil {
				t.Fatal(err)
			}
			lines = append(lines, string(line))
		}
		put(t, filepath.Join(env.Projects, fmt.Sprintf("slug-%d", i%5), id+".jsonl"), strings.Join(lines, "\n")+"\n")
	}
}

// sampleChecks are a doctor's report with every level, long lines and paths, in the current language.
func sampleChecks() []ops.Check {
	return []ops.Check{
		{Level: ops.OK, Title: i18n.T("ops.doctor.install", "kind", i18n.T("ops.kind.msix")),
			Detail: `C:\Users\me\AppData\Local\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude`},
		{Level: ops.Info, Title: i18n.T("ops.doctor.running")},
		{Level: ops.OK, Title: i18n.T("ops.doctor.signed-in", "name", "work")},
		{Level: ops.Info, Title: i18n.T("ops.doctor.profiles", "n", 3), Detail: "client, personal, work"},
		{Level: ops.Fail, Title: i18n.T("ops.doctor.list", "label", "personal", "n", 97),
			Detail: i18n.T("ops.doctor.linked"), Fix: i18n.T("ops.doctor.linked.fix")},
		{Level: ops.Warn, Title: i18n.T("ops.doctor.lost", "n", 6), Fix: i18n.T("ops.doctor.lost.fix")},
		{Level: ops.Info, Title: i18n.T("ops.doctor.terminal-cleanup", "n", 30),
			Fix: i18n.T("ops.doctor.terminal-cleanup.fix", "path", "~/.claude/settings.json")},
	}
}

// deliver runs cmd and hands what it returns to the app, each message of a batch too. The usage
// read is followed to its end: each of its messages brings the command that waits for the next.
func deliver(a *app, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			deliver(a, c)
		}
	case usageProgressMsg:
		_, next := a.Update(msg)
		deliver(a, next)
	default:
		a.Update(msg)
	}
}

// openActivity opens the Activity tab and delivers the history it reads.
func openActivity(a *app) *activityScreen {
	deliver(a, press(a, activityTab))
	return a.screens[2].(*activityScreen)
}

// openUsage opens the Usage tab and runs its read to the end.
func openUsage(a *app) *usageScreen {
	deliver(a, press(a, usageTab))
	return a.screens[3].(*usageScreen)
}

var namedKeys = map[string]rune{
	"up": tea.KeyUp, "down": tea.KeyDown, "home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
}

// tap presses keys like press, and named keys (down, end...) as a terminal sends them.
func tap(a *app, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		if code, ok := namedKeys[k]; ok {
			_, cmd = a.Update(tea.KeyPressMsg{Code: code})
		} else {
			cmd = press(a, k)
		}
	}
	return cmd
}

func frameText(a *app) string { return strings.Join(screen(a), "\n") }

func wantIn(t *testing.T, a *app, texts ...string) {
	t.Helper()
	frame := frameText(a)
	for _, s := range texts {
		if !strings.Contains(frame, a.L().Text(s)) {
			t.Errorf("frame lacks %q:\n%s", s, frame)
		}
	}
}

// utcClock shows every time in UTC until the test ends, so frames read the same in any time zone.
func utcClock(t *testing.T) {
	local := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = local })
}

func TestInsightsFrames(t *testing.T) {
	for _, tab := range []struct {
		name string
		open func(t *testing.T, a *app)
	}{
		{"activity", func(t *testing.T, a *app) {
			activityFixture(t, a.Env)
			openActivity(a)
		}},
		{"usage", func(t *testing.T, a *app) {
			usageFixture(t, a.Env)
			openUsage(a)
		}},
		{"doctor", func(t *testing.T, a *app) {
			press(a, doctorTab)
			a.Update(doctorMsg(sampleChecks()))
		}},
		{"activity_details", func(t *testing.T, a *app) {
			activityFixture(t, a.Env)
			openActivity(a)
			tap(a, "down", "down", "enter")
		}},
	} {
		for _, size := range [][2]int{{80, 24}, {120, 40}} {
			t.Run(fmt.Sprintf("%s/en_%dx%d", tab.name, size[0], size[1]), func(t *testing.T) {
				utcClock(t)
				env, _ := fixture(t)
				a := newTestApp(t, env, "en", size[0], size[1])
				tab.open(t, a)
				checkFrame(t, a)
				golden.RequireEqual(t, frameText(a))
			})
		}
	}
}

func TestActivityStates(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	press(a, activityTab) // the history is not read yet
	wantIn(t, a, i18n.T("state.loading"))

	deliver(a, a.screens[2].(*activityScreen).read(a.Ctx))
	wantIn(t, a, i18n.T("insights.activity.empty.title"))
	if keys := a.screens[2].Keys(a.Ctx); len(keys) != 0 {
		t.Errorf("an empty history offers keys: %v", keys)
	}
	if press(a, "u"); a.toast.text != i18n.T("ops.err.nothing-to-undo") || a.dialog != nil {
		t.Errorf("u with nothing to undo: toast %q, dialog %v", a.toast.text, a.dialog)
	}

	activityFixture(t, env)
	openActivity(a)
	checkFrame(t, a)
	wantIn(t, a, i18n.T("ops.done.merge", "n", 2, "to", "work"), i18n.T("insights.activity.undone"),
		i18n.T("time.minutes_ago", "n", 20), i18n.Date(now.Add(-45*24*time.Hour).Local()))

	a.Update(historyMsg{err: errors.New("journal is locked")})
	wantIn(t, a, i18n.T("insights.activity.error"), "journal is locked")
}

// enter shows a change's details over the list: its kind, counts and file names; esc closes them.
func TestActivityDetails(t *testing.T) {
	env, _ := fixture(t)
	activityFixture(t, env)
	a := newTestApp(t, env, "en", 80, 24)
	s := openActivity(a)
	if tap(a, "down", "down", "enter"); !s.open {
		t.Fatal("enter should open the details")
	}
	checkFrame(t, a)
	wantIn(t, a, i18n.T("insights.action.move"), i18n.T("insights.details.counts", "created", 3, "removed", 3),
		"local_00000004-0000-4000-8000-000000000000.json")
	if press(a, "esc"); s.open || a.tab != 2 {
		t.Errorf("esc should close the details and stay on Activity; open %v, tab %d", s.open, a.tab)
	}
}

// Many files in a short window: the names that do not fit are counted instead.
func TestActivityDetailsCountsTheRest(t *testing.T) {
	env, _ := fixture(t)
	activityFixture(t, env)
	a := newTestApp(t, env, "en", 60, 18)
	s := openActivity(a)
	tap(a, "end", "enter")
	if !s.open {
		t.Fatal("enter should open the details of the oldest change")
	}
	checkFrame(t, a)
	frame := frameText(a)
	shown := strings.Count(frame, "local_")
	if want := i18n.T("insights.details.more", "n", 12-shown); shown == 0 || !strings.Contains(frame, want) {
		t.Errorf("%d names shown, want some and %q:\n%s", shown, want, frame)
	}
}

// u undoes the newest change that is not undone, after asking, with Claude Desktop closed around it;
// the timeline then shows it undone and the files are back.
func TestActivityUndo(t *testing.T) {
	env, desk := fixture(t)
	work, err := env.FindList("work")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := env.PlanMerge(work)
	if err != nil {
		t.Fatal(err)
	}
	res, err := env.Apply(plan)
	if err != nil {
		t.Fatal(err)
	}
	cards := filepath.Join(env.Install.DataDir, "claude-code-sessions", workID, orgID)
	count := func() int {
		entries, err := os.ReadDir(cards)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	if count() != 5 {
		t.Fatalf("the merge left %d cards in work's list, want 5", count())
	}
	desk.running = true

	a := newTestApp(t, env, "en", 100, 30)
	s := openActivity(a)
	press(a, "u")
	if a.dialog == nil || !strings.Contains(a.dialog.body, res.Summary) {
		t.Fatalf("u should ask first, naming the change; dialog %+v", a.dialog)
	}
	press(a, "y")
	if a.runner == nil {
		t.Fatal("agreeing should start the change")
	}
	a.Update(a.runner.try()()) // ops refuses while Desktop is open
	a.Update(quitMsg{desk.Quit()})
	if s.Update(a.Ctx, statusMsg{}) != nil {
		t.Error("the history was read again while the undo was still running")
	}
	a.Update(closedMsg{nil})
	a.Update(a.runner.try()())
	a.Update(launchedMsg{desk.Launch()})
	if a.runner != nil || a.toast.text != i18n.T("insights.undo.done", "summary", res.Summary) {
		t.Fatalf("the undo should finish with a toast; runner %v, toast %q", a.runner, a.toast.text)
	}
	deliver(a, s.Update(a.Ctx, statusMsg{})) // the change is over: the history is read again

	if !s.js[0].Undone || count() != 3 {
		t.Errorf("after undo: newest undone %v, %d cards in work's list; want true and 3", s.js[0].Undone, count())
	}
	if quits, forces, launches := desk.count(); quits != 1 || forces != 0 || launches != 1 {
		t.Errorf("quits %d, forces %d, launches %d; want 1, 0, 1", quits, forces, launches)
	}
	wantIn(t, a, i18n.T("insights.activity.undone"))
	if press(a, "u"); a.dialog != nil || a.toast.text != i18n.T("ops.err.nothing-to-undo") {
		t.Errorf("with everything undone u should say so; dialog %v, toast %q", a.dialog, a.toast.text)
	}
}

// Clicking a row selects it, clicking it again opens its details, and the wheel moves the cursor.
func TestActivityMouse(t *testing.T) {
	env, _ := fixture(t)
	activityFixture(t, env)
	a := newTestApp(t, env, "en", 100, 30)
	s := openActivity(a)
	a.View() // clicks land on what was drawn
	click := func(row int) { a.Update(tea.MouseClickMsg{X: 10, Y: 2 + listTop + row, Button: tea.MouseLeft}) }
	click(2)
	if s.cursor != 2 || s.open {
		t.Fatalf("a click on row 2 should select it; cursor %d, open %v", s.cursor, s.open)
	}
	click(2)
	if !s.open {
		t.Fatal("a click on the selected row should open its details")
	}
	click(0)
	if s.open || s.cursor != 2 {
		t.Fatalf("a click should close the details and keep the cursor; open %v, cursor %d", s.open, s.cursor)
	}
	a.Update(tea.MouseWheelMsg{X: 10, Y: 5, Button: tea.MouseWheelDown})
	if s.cursor != 3 {
		t.Errorf("the wheel moved the cursor to %d, want 3", s.cursor)
	}
}

// The read reports its progress as it goes; the screen shows it until the dashboard is ready.
func TestUsageProgress(t *testing.T) {
	env, _ := fixture(t)
	usageFixture(t, env)
	a := newTestApp(t, env, "en", 100, 30)
	s := a.screens[3].(*usageScreen)
	var progress []usageProgressMsg
	for cmd := s.read(a.Ctx); cmd != nil; {
		msg := cmd()
		if p, ok := msg.(usageProgressMsg); ok {
			progress = append(progress, p)
		}
		cmd = s.Update(a.Ctx, msg)
	}
	if len(progress) == 0 {
		t.Fatal("no progress was reported")
	}
	for _, p := range progress {
		if p.total != 26 || p.done < 0 || p.done > p.total {
			t.Errorf("progress %d of %d, want some of 26", p.done, p.total)
		}
	}
	if !s.loaded || s.stats.Sessions != 26 {
		t.Fatalf("loaded %v with %d sessions, want 26", s.loaded, s.stats.Sessions)
	}

	press(a, usageTab)
	s.loading, s.done, s.total = true, 13, 26
	wantIn(t, a, i18n.T("insights.usage.reading", "n", 26, "pct", "50%"))
	s.total = 0
	wantIn(t, a, i18n.T("insights.usage.finding"))
	checkFrame(t, a)

	// r reads again; the cache makes it quick and the numbers stay the same.
	s.loading = false
	deliver(a, press(a, "r"))
	if s.loading || s.stats.Sessions != 26 || s.stats.FilesRead != 0 {
		t.Errorf("after r: loading %v, %d sessions, %d files read again; want false, 26, 0", s.loading, s.stats.Sessions, s.stats.FilesRead)
	}
}

func TestUsageStates(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	s := openUsage(a)
	wantIn(t, a, i18n.T("insights.usage.empty.title"))

	s.Update(a.Ctx, usageMsg{err: errors.New("disk on fire")})
	wantIn(t, a, i18n.T("insights.usage.error"), "disk on fire", i18n.T("insights.usage.retry"))

	usageFixture(t, env)
	deliver(a, press(a, "r"))
	wantIn(t, a, i18n.T("insights.usage.total"), i18n.T("insights.usage.by_model"), "opus 5", "sonnet 4.5",
		"haiku 4.5", "notes", i18n.T("insights.usage.days", "n", 30))
	checkFrame(t, a)
}

// A short window scrolls the dashboard; nothing drawn is ever wider than the window.
func TestUsageScrolls(t *testing.T) {
	env, _ := fixture(t)
	usageFixture(t, env)
	a := newTestApp(t, env, "en", 60, 18)
	s := openUsage(a)
	checkFrame(t, a)
	if s.scroll.max == 0 {
		t.Fatal("the dashboard should not fit 60x18")
	}
	tap(a, "end")
	checkFrame(t, a)
	wantIn(t, a, i18n.T("insights.usage.projects"))
}

func TestDoctorStates(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	s := a.screens[4].(*doctorScreen)
	if cmd := press(a, doctorTab); cmd == nil || !s.loading {
		t.Fatal("opening Doctor should start the checks")
	}
	wantIn(t, a, i18n.T("insights.doctor.checking"))

	a.Update(doctorMsg(env.Doctor()))
	wantIn(t, a, i18n.T("insights.doctor.fine"), i18n.T("ops.doctor.signed-in", "name", "work"))

	a.Update(doctorMsg(sampleChecks()))
	wantIn(t, a, i18n.T("insights.doctor.attention", "n", 2), i18n.T("ops.doctor.linked.fix"))
	if frame := frameText(a); !strings.Contains(frame, "✗ "+i18n.T("ops.doctor.list", "label", "personal", "n", 97)) {
		t.Errorf("a failed check should read ✗ and its title:\n%s", frame)
	}

	press(a, "r")
	if !s.loading {
		t.Error("r should run the checks again")
	}
	wantIn(t, a, i18n.T("insights.doctor.checking"), i18n.T("ops.doctor.linked.fix")) // the last report stays meanwhile

	a.Update(doctorMsg(nil))
	wantIn(t, a, i18n.T("insights.doctor.none"))
}

func TestDoctorScrolls(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 60, 18)
	s := a.screens[4].(*doctorScreen)
	press(a, doctorTab)
	a.Update(doctorMsg(sampleChecks()))
	if frame := frameText(a); s.scroll.max == 0 || !strings.Contains(frame, i18n.T("insights.key.scroll")) {
		t.Fatalf("a long report should scroll and say so; max %d", s.scroll.max)
	}
	a.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if s.scroll.top != 3 {
		t.Errorf("the wheel scrolled to %d, want 3", s.scroll.top)
	}
	tap(a, "end")
	checkFrame(t, a)
	wantIn(t, a, i18n.T("insights.doctor.attention", "n", 2), i18n.T("ops.doctor.terminal-cleanup", "n", 30))
}

// Each level has its icon; the summary takes the worst one, counts what needs attention and
// tallies every level.
func TestDoctorLevels(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 100, 30)
	press(a, doctorTab)
	icons := map[ops.Level]string{ops.OK: "✓", ops.Info: "i", ops.Warn: "!", ops.Fail: "✗"}
	for _, tc := range []struct {
		levels       []ops.Level
		verdict, sum string
	}{
		{[]ops.Level{ops.OK, ops.Info, ops.OK}, "✓ " + i18n.T("insights.doctor.fine"), "✓ 2   i 1"},
		{[]ops.Level{ops.Info, ops.Warn}, "! " + i18n.T("insights.doctor.attention", "n", 1), "i 1   ! 1"},
		{[]ops.Level{ops.Warn, ops.Fail, ops.OK}, "✗ " + i18n.T("insights.doctor.attention", "n", 2), "✓ 1   ! 1   ✗ 1"},
	} {
		var checks []ops.Check
		for i, lv := range tc.levels {
			checks = append(checks, ops.Check{Level: lv, Title: fmt.Sprintf("check %d", i)})
		}
		a.Update(doctorMsg(checks))
		rows := screen(a)
		if head := strings.TrimSpace(rows[3]); !strings.HasPrefix(head, tc.verdict) || !strings.HasSuffix(head, tc.sum) {
			t.Errorf("%v: summary %q, want %q ... %q", tc.levels, head, tc.verdict, tc.sum)
		}
		for i, lv := range tc.levels {
			if want := fmt.Sprintf("%s check %d", icons[lv], i); !strings.Contains(frameText(a), want) {
				t.Errorf("%v: no %q in the report", tc.levels, want)
			}
		}
	}
}

// A single reply today fills only the chart's last column: today is at the right edge.
func TestUsageChartToday(t *testing.T) {
	env, _ := fixture(t)
	a := newTestApp(t, env, "en", 80, 24)
	s := &usageScreen{stats: transcript.Stats{ByDay: map[string]transcript.Usage{now.Local().Format(time.DateOnly): {Output: 500}}}}
	chart := s.chart(a.Ctx, 78, false)
	for _, row := range chart[1:5] { // the columns, under the title
		cells := []rune(ansi.Strip(row))
		if cells[len(cells)-1] != '█' || strings.TrimSpace(string(cells[:len(cells)-1])) != "" {
			t.Errorf("only the last column should be filled: %q", string(cells))
		}
	}
	if dates := ansi.Strip(chart[len(chart)-1]); !strings.HasSuffix(dates, i18n.Date(now.Local())) {
		t.Errorf("today's date should close the chart at the right: %q", dates)
	}
}

// Every screen fills the window exactly at any size the app draws, with no row wider than it.
func TestInsightsFitAnySize(t *testing.T) {
	env, _ := fixture(t)
	activityFixture(t, env)
	usageFixture(t, env)
	a := newTestApp(t, env, "en", 60, 18)
	s := openActivity(a)
	openUsage(a)
	press(a, doctorTab)
	a.Update(doctorMsg(sampleChecks()))
	sweep := func() {
		for w := 60; w <= 200; w += 7 {
			for h := 18; h <= 50; h += 8 {
				a.Update(tea.WindowSizeMsg{Width: w, Height: h})
				checkFrame(t, a)
			}
		}
	}
	for _, tab := range []string{activityTab, usageTab, doctorTab} {
		press(a, tab)
		sweep()
	}
	press(a, activityTab)
	if press(a, "enter"); !s.open {
		t.Fatal("enter should open the details")
	}
	sweep()
}
