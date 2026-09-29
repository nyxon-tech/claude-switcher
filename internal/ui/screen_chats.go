package ui

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
	"github.com/sahilm/fuzzy"
)

// chatsScreen browses the chats of every list: a chip per list (and one for lost chats), a
// search field, the list and a preview of the chat under the cursor. It copies, moves, exports
// and recovers chats; every write goes through c.Change.
type chatsScreen struct {
	started  bool
	input    textinput.Model
	typing   bool // the search field has focus
	spin     spinner.Model
	spinning bool

	gen, lostGen int  // loads started: an older result is dropped
	loading      bool // the lists are being read
	loaded       bool // the lists have been read at least once
	err          error
	stale        bool   // a change ran: read everything again once it is done
	sig          string // the lists as last read, to notice changes in c.Status
	lists        []ops.List
	byList       map[string][]chatRow // by list folder
	all          []chatRow            // every list, newest first
	lost         []chatRow
	hidden       int // older parts of chats that Orphans left out

	chip        string          // chipAll, chipLost or a list folder
	shown       []int           // indexes into rows() that match the search, in order
	cursor, top int             // cursor and first row drawn, both in shown
	selected    map[string]bool // row keys of the chip's selected rows

	previews   map[string]chatPreview // by previewKey
	previewSeq int                    // cursor moves: only the newest debounce tick reads
	reading    string                 // previewKey being read

	full     bool // the preview fills the body
	pager    viewport.Model
	pagerKey string // what the pager holds

	downloads string // where exports go; "" is the user's Downloads folder
	geo       chatsGeo
}

// chatRow is one chat as the list shows it, with its search text folded once.
type chatRow struct {
	ops.Chat
	key    string // unique across lists: the card file, or a lost chat's transcript
	hay    string // folded title and project
	hayAll string // hay and the list label, for All
	lost   bool
}

type chatPreview struct {
	meta transcript.Meta
	msgs []transcript.Message
	err  error
}

type (
	chatsLoadedMsg struct {
		gen   int
		lists []ops.List
		chats [][]ops.Chat // per list
		err   error
	}
	chatsLostMsg struct {
		gen     int
		orphans []ops.Orphan
		hidden  int
		err     error
	}
	chatsTickMsg    struct{ seq int } // the cursor rested: read the preview
	chatsPreviewMsg struct {
		key string
		p   chatPreview
	}
	chatsPlanMsg struct {
		plan ops.Plan
		err  error
	}
	chatsExportMsg struct {
		paths []string
		err   error
	}
)

const (
	chipAll  = ""
	chipLost = "lost" // list chips are folders, which never read "lost"
)

func (*chatsScreen) Title() string  { return i18n.T("tab.chats") }
func (s *chatsScreen) Typing() bool { return s.typing }

func (s *chatsScreen) start() {
	s.started = true
	s.input = textinput.New()
	s.input.SetVirtualCursor(false) // the field is drawn through the layout, cursor included
	s.input.CharLimit = 120
	// Editing happens at the end only: the drawn field has no cursor to move.
	km := &s.input.KeyMap
	for _, b := range []*key.Binding{&km.CharacterForward, &km.CharacterBackward, &km.WordForward, &km.WordBackward,
		&km.LineStart, &km.LineEnd, &km.DeleteCharacterForward, &km.DeleteAfterCursor, &km.DeleteWordForward,
		&km.AcceptSuggestion, &km.NextSuggestion, &km.PrevSuggestion} {
		b.SetEnabled(false)
	}
	s.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot)) // drawn through spinner(c)
	s.pager = viewport.New()
	s.pager.KeyMap.Left.SetEnabled(false)
	s.pager.KeyMap.Right.SetEnabled(false)
	s.byList, s.selected, s.previews = map[string][]chatRow{}, map[string]bool{}, map[string]chatPreview{}
}

func (s *chatsScreen) Update(c *Ctx, msg tea.Msg) tea.Cmd {
	if !s.started {
		s.start()
	}
	switch msg := msg.(type) {
	case shownMsg:
		s.blur()
		return tea.Batch(s.load(c), s.loadLost(c), s.spinCmd())
	case restyleMsg:
		s.pagerKey = "" // the full preview is drawn again, in the new theme and rtl mode
	case statusMsg:
		return s.statusChanged(c)
	case chatsLoadedMsg:
		return s.loadedChats(c, msg)
	case chatsLostMsg:
		return s.loadedLost(msg)
	case chatsTickMsg:
		if msg.seq == s.previewSeq {
			return s.readPreview(c)
		}
	case chatsPreviewMsg:
		s.previews[msg.key] = msg.p
		if s.reading == msg.key {
			s.reading = ""
		}
	case chatsPlanMsg:
		return s.confirmTransfer(c, msg)
	case chatsExportMsg:
		return s.exported(c, msg)
	case spinner.TickMsg:
		return s.spinTick(msg)
	case tea.KeyPressMsg:
		return s.key(c, msg)
	case tea.MouseClickMsg:
		return s.click(c, msg)
	case tea.MouseWheelMsg:
		return s.wheel(msg)
	case tea.PasteMsg:
		if s.typing {
			return s.edit(msg)
		}
	}
	return nil
}

// load reads every list and its chats in the background.
func (s *chatsScreen) load(c *Ctx) tea.Cmd {
	s.gen++
	s.loading = true
	gen, env := s.gen, c.Env
	return func() tea.Msg {
		msg := chatsLoadedMsg{gen: gen}
		msg.lists, msg.err = env.Lists()
		for _, l := range msg.lists {
			if msg.err != nil {
				break
			}
			var chats []ops.Chat
			chats, msg.err = env.Chats(l)
			msg.chats = append(msg.chats, chats)
		}
		return msg
	}
}

// loadLost looks for lost chats in the background.
func (s *chatsScreen) loadLost(c *Ctx) tea.Cmd {
	s.lostGen++
	gen, env := s.lostGen, c.Env
	return func() tea.Msg {
		orphans, hidden, err := env.Orphans()
		return chatsLostMsg{gen, orphans, hidden, err}
	}
}

// statusChanged reads the lists again when a change ran here or anything else changed them.
func (s *chatsScreen) statusChanged(c *Ctx) tea.Cmd {
	if !s.loaded || s.loading || c.runner != nil {
		return nil
	}
	if !s.stale && chatsSig(c.Status.Lists) == s.sig {
		return nil
	}
	s.stale = false
	return tea.Batch(s.load(c), s.loadLost(c))
}

// chatsSig identifies the lists and their chat counts.
func chatsSig(lists []ops.List) string {
	var b strings.Builder
	for _, l := range lists {
		b.WriteString(l.Dir + "|" + l.Label + "|" + strconv.Itoa(l.Chats) + "\n")
	}
	return b.String()
}

func (s *chatsScreen) loadedChats(c *Ctx, msg chatsLoadedMsg) tea.Cmd {
	if msg.gen != s.gen {
		return nil
	}
	s.loading = false
	if msg.err != nil {
		s.err = msg.err
		if s.loaded { // keep showing what was read before
			return c.Toast(toastFail, msg.err.Error())
		}
		return nil
	}
	cur := s.currentKey()
	s.err, s.loaded = nil, true
	s.lists, s.sig = msg.lists, chatsSig(msg.lists)
	s.byList, s.all = make(map[string][]chatRow, len(msg.lists)), nil
	for i, l := range msg.lists {
		rows := make([]chatRow, len(msg.chats[i]))
		for j, ch := range msg.chats[i] {
			hay := rtl.Fold(ch.Title + " " + ch.Project)
			rows[j] = chatRow{Chat: ch, key: ch.Path, hay: hay, hayAll: hay + " " + rtl.Fold(l.Label)}
		}
		s.byList[l.Dir] = rows
		s.all = append(s.all, rows...)
	}
	slices.SortStableFunc(s.all, func(a, b chatRow) int { return b.Last.Compare(a.Last) })
	return s.refresh(cur)
}

func (s *chatsScreen) loadedLost(msg chatsLostMsg) tea.Cmd {
	if msg.gen != s.lostGen {
		return nil
	}
	cur := s.currentKey()
	s.lost, s.hidden = nil, msg.hidden
	if msg.err != nil { // the doctor says why; the Lost chip just stays away
		s.hidden = 0
	}
	for _, o := range msg.orphans {
		m := o.Meta
		hay := rtl.Fold(o.Title + " " + o.Project)
		s.lost = append(s.lost, chatRow{Chat: ops.Chat{
			Title: o.Title, Project: o.Project, Cwd: m.Cwd, Session: m.Session, Model: m.Model,
			Created: m.Start, Last: m.End, HasHistory: true, Transcript: m.Path,
		}, key: "lost:" + m.Session, hay: hay, hayAll: hay, lost: true})
	}
	if s.chip == chipLost {
		return s.refresh(cur)
	}
	return nil
}

// refresh redoes the search after the rows changed, keeping the cursor on the chat it was on
// and the selection of chats that are still there.
func (s *chatsScreen) refresh(cur string) tea.Cmd {
	if _, ok := s.byList[s.chip]; !ok && s.chip != chipAll && s.chip != chipLost {
		s.chip = chipAll
	}
	rows := s.rows()
	keep := make(map[string]bool, len(s.selected))
	for _, r := range rows {
		if s.selected[r.key] {
			keep[r.key] = true
		}
	}
	s.selected = keep
	s.filter()
	s.cursor = 0
	for i, idx := range s.shown {
		if rows[idx].key == cur {
			s.cursor = i
		}
	}
	return s.moved()
}

// rows are the chip's chats, before the search.
func (s *chatsScreen) rows() []chatRow {
	switch s.chip {
	case chipAll:
		return s.all
	case chipLost:
		return s.lost
	}
	return s.byList[s.chip]
}

func (s *chatsScreen) current() (chatRow, bool) {
	if s.cursor < 0 || s.cursor >= len(s.shown) {
		return chatRow{}, false
	}
	return s.rows()[s.shown[s.cursor]], true
}

func (s *chatsScreen) currentKey() string {
	r, _ := s.current()
	return r.key
}

// filter keeps the rows that match the search: a substring for one letter, fuzzy from two on,
// over text folded so that Persian matches with or without ZWNJ and with Arabic ي or ك.
func (s *chatsScreen) filter() {
	rows := s.rows()
	q := rtl.Fold(strings.TrimSpace(s.input.Value()))
	s.shown = s.shown[:0]
	hays := chatHays{rows, s.chip == chipAll}
	switch {
	case q == "":
		for i := range rows {
			s.shown = append(s.shown, i)
		}
	case utf8.RuneCountInString(q) == 1:
		for i := range rows {
			if strings.Contains(hays.String(i), q) {
				s.shown = append(s.shown, i)
			}
		}
	default:
		// Rank by how well the letters match, not by length: fuzzy takes a point off for every
		// byte left over, which would put short titles, and English before Persian, first.
		// Equal matches stay newest first.
		matches := fuzzy.FindFromNoSort(q, hays)
		score := func(m fuzzy.Match) int { return m.Score + len(m.Str) - len(m.MatchedIndexes) }
		slices.SortStableFunc(matches, func(a, b fuzzy.Match) int { return score(b) - score(a) })
		for _, m := range matches {
			s.shown = append(s.shown, m.Index)
		}
	}
}

// chatHays is the search text of rows, for fuzzy.
type chatHays struct {
	rows  []chatRow
	label bool
}

func (h chatHays) Len() int { return len(h.rows) }

func (h chatHays) String(i int) string {
	if h.label {
		return h.rows[i].hayAll
	}
	return h.rows[i].hay
}

// dirOf is the direction of user text as UAX #9 finds it: that of its first strong letter.
func dirOf(s string) rtl.Dir {
	for _, r := range s {
		switch {
		case rtl.HasRTL(string(r)):
			return rtl.RTL
		case unicode.IsLetter(r):
			return rtl.LTR
		}
	}
	return rtl.LTR
}

// spinner is the spinner's frame in the theme's accent.
func (s *chatsScreen) spinner(c *Ctx) string { return c.St.Accent.Render(s.spin.View()) }

// spinCmd starts the spinner unless it turns already.
func (s *chatsScreen) spinCmd() tea.Cmd {
	if s.spinning {
		return nil
	}
	s.spinning = true
	return s.spin.Tick
}

// spinTick turns the spinner while something is read, and lets it rest after.
func (s *chatsScreen) spinTick(msg spinner.TickMsg) tea.Cmd {
	if msg.ID != s.spin.ID() {
		return nil
	}
	if !s.loading && s.reading == "" {
		s.spinning = false
		return nil
	}
	var cmd tea.Cmd
	s.spin, cmd = s.spin.Update(msg)
	return cmd
}
