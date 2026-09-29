package ops

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/claude"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

func lost(n int) string { return "20000000-0000-4000-8000-00000000000" + string(rune('0'+n)) }

func day(d int) time.Time { return time.Date(2026, 9, d, 10, 0, 0, 0, time.UTC) }

// newCards reads the cards of dir other than the fixture's own.
func newCards(t *testing.T, dir string) []*claude.Card {
	t.Helper()
	var out []*claude.Card
	cards, err := claude.Space{Dir: dir}.Cards()
	must(t, err)
	for _, c := range cards {
		if c.ID() != "local_c1" && c.ID() != "local_c2" {
			out = append(out, c)
		}
	}
	return out
}

func TestRescue(t *testing.T) {
	f, _ := chatFixture(t)
	workDir := f.listDir(work)
	persian := "رفع باگ ترجمه"
	site := filepath.FromSlash("/work/site")
	f.metas[lost(1)] = transcript.Meta{Session: lost(1), Cwd: proj, Title: persian, FirstPrompt: "fix the translation bug",
		Model: "claude-opus-5", Start: day(20), End: day(20).Add(5 * time.Minute)}
	f.metas[lost(2)] = transcript.Meta{Session: lost(2), Cwd: site, FirstPrompt: "build the login page", Start: day(21), End: day(21)}

	orphans, hidden, err := f.Orphans()
	must(t, err)
	if len(orphans) != 2 || hidden != 0 || orphans[0].Meta.Session != lost(2) || orphans[1].Title != persian {
		t.Fatalf("rescue should list only chats no list has, newest first: %+v, hidden %d", orphans, hidden)
	}

	res, err := f.Rescue(f.list("work"), []string{lost(1), strings.ToUpper(lost(2))})
	must(t, err)
	made := newCards(t, workDir)
	if res.Created != 2 || len(made) != 2 {
		t.Fatalf("rescue should write one card per chat, got %d", len(made))
	}
	byID := map[string]*claude.Card{}
	for _, c := range made {
		byID[c.Session()] = c
		if data, _ := os.ReadFile(c.Path); bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
			t.Error("cards must have no byte order mark: JSON.parse rejects it")
		}
	}
	first, second := byID[lost(1)], byID[lost(2)]
	if first.Title() != persian || second.Title() != "build the login page" {
		t.Errorf("titles %q, %q", first.Title(), second.Title())
	}
	if first.Cwd() != proj || first.Model() != "claude-opus-5" || first.Created() != day(20).UnixMilli() || first.LastActivity() != day(20).Add(5*time.Minute).UnixMilli() || second.Model() != "" {
		t.Error("cwd, model and times should come from the transcript")
	}
	if orphans, _, _ := f.Orphans(); len(orphans) != 0 {
		t.Error("nothing should be left to rescue")
	}
	_, err = f.Undo()
	must(t, err)
	if len(newCards(t, workDir)) != 0 {
		t.Error("undo should remove the recovered cards")
	}
}

// The v2 "rescue keeps one chat as one chat" fixture, plus priorCliSessionIds.
func TestRescueOffersEachLostChatOnce(t *testing.T) {
	f, now := chatFixture(t)
	workDir := f.listDir(work)
	s9 := "10000000-0000-4000-8000-000000000009"
	f.card(work, "c9", s9, "Big feature", now, `,"priorCliSessionIds":["`+lost(7)+`"]`)
	f.transcript(s9, "Big feature", "", day(22))
	f.transcript(lost(3), "Big feature", "", day(21))        // an older part titled like a listed chat
	f.transcript(lost(4), "Lost chat", "", day(10))          // a lost chat in two parts
	f.transcript(lost(5), "Lost chat", "", day(11))          //
	f.transcript(lost(6), "Deleted on purpose", "", day(12)) // deleted in the app
	f.put(filepath.Join(workDir, "deleted_"+lost(6)), "1789000000000")
	f.transcript(lost(7), "Renamed since", "", day(9))   // named by c9's priorCliSessionIds
	f.metas[lost(8)] = transcript.Meta{Session: lost(8)} // metadata only, no conversation

	orphans, hidden, err := f.Orphans()
	must(t, err)
	if len(orphans) != 1 || orphans[0].Meta.Session != lost(5) || orphans[0].Title != "Lost chat" {
		t.Fatalf("only the newest part of the lost chat should be offered: %+v", orphans)
	}
	if hidden != 2 {
		t.Errorf("hidden = %d, want 2 (the Big feature part and the older Lost chat part)", hidden)
	}
	for _, id := range []string{lost(3), lost(6), lost(7)} {
		_, err := f.Rescue(f.list("work"), []string{id})
		wantCode(t, err, ChatNotFound)
	}

	res, err := f.Rescue(f.list("work"), []string{lost(5)})
	must(t, err)
	var titles []string
	for _, c := range newCards(t, workDir) {
		titles = append(titles, c.Title()+"="+c.Session())
	}
	sort.Strings(titles)
	if res.Created != 1 || strings.Join(titles, ",") != "Big feature="+s9+",Lost chat="+lost(5) {
		t.Fatalf("cards = %v", titles)
	}
}

// Rescue, the chat list, preview and export on real transcript files, read by the transcript
// package instead of fakes.
func TestRealTranscripts(t *testing.T) {
	f, _ := chatFixture(t)
	f.Index, f.QuickMeta, f.ReadMeta = transcript.Index, transcript.QuickMeta, transcript.ReadMeta
	persian := "رفع باگ ترجمه"
	line := func(typ, ts, message string) string {
		return `{"type":"` + typ + `","uuid":"u-` + ts + `","timestamp":"` + ts + `","cwd":` + str(proj) + `,"sessionId":"` + lost(1) + `","message":` + message + `}`
	}
	dir := filepath.Join(f.Projects, "C--work-proj")
	f.put(filepath.Join(dir, lost(1)+".jsonl"), strings.Join([]string{
		line("user", "2026-09-20T10:00:00.000Z", `{"role":"user","content":"fix the translation bug"}`),
		line("assistant", "2026-09-20T10:05:00.000Z", `{"id":"m1","model":"claude-opus-5","role":"assistant","content":[{"type":"text","text":"done"}]}`),
		`{"type":"custom-title","customTitle":"` + persian + `","sessionId":"` + lost(1) + `"}`,
	}, "\n")+"\n")
	f.put(filepath.Join(dir, lost(2)+".jsonl"), `{"type":"custom-title","customTitle":"metadata only","sessionId":"`+lost(2)+`"}`+"\n")

	orphans, _, err := f.Orphans()
	must(t, err)
	if len(orphans) != 1 || orphans[0].Title != persian {
		t.Fatalf("only the transcript with a conversation is lost: %+v", orphans)
	}
	_, err = f.Rescue(f.list("work"), []string{lost(1)})
	must(t, err)
	chats, err := f.Chats(f.list("work"))
	must(t, err)
	i := slices.IndexFunc(chats, func(c Chat) bool { return c.Session == lost(1) })
	if i < 0 {
		t.Fatal("the rescued chat is not listed")
	}
	c := chats[i]
	if c.Title != persian || !c.HasHistory || c.Model != "claude-opus-5" || !c.Created.Equal(day(20)) || !c.Last.Equal(day(20).Add(5*time.Minute)) {
		t.Errorf("chat = %+v", c)
	}

	m, msgs, err := f.Preview(c)
	must(t, err)
	if m.Title != persian || m.Prompts != 1 || len(msgs) != 2 || msgs[1].Text != "done" {
		t.Errorf("preview = %+v, %+v", m, msgs)
	}
	var md bytes.Buffer
	must(t, f.Export(c, "md", &md))
	if !strings.Contains(md.String(), persian) || !strings.Contains(md.String(), "fix the translation bug") {
		t.Errorf("markdown export:\n%s", md.String())
	}
}
