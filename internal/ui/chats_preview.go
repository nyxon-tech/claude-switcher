package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// chatsDebounce is how long the cursor rests on a chat before its transcript is read, so
// scrolling through the list reads nothing.
const chatsDebounce = 150 * time.Millisecond

// previewKey names a chat's preview; a chat used since gets a new one.
func previewKey(r chatRow) string { return r.key + "@" + strconv.FormatInt(r.Last.UnixMilli(), 10) }

// moved is called after the cursor landed on another chat: its preview is read once the
// cursor rests there, unless it was read before.
func (s *chatsScreen) moved() tea.Cmd {
	s.previewSeq++
	r, ok := s.current()
	if !ok {
		return nil
	}
	if _, done := s.previews[previewKey(r)]; done {
		return nil
	}
	seq := s.previewSeq
	return tea.Tick(chatsDebounce, func(time.Time) tea.Msg { return chatsTickMsg{seq} })
}

// readPreview reads the preview of the chat under the cursor in the background.
func (s *chatsScreen) readPreview(c *Ctx) tea.Cmd {
	r, ok := s.current()
	if !ok {
		return nil
	}
	k := previewKey(r)
	if _, done := s.previews[k]; done || s.reading == k {
		return nil
	}
	s.reading = k
	env, ch := c.Env, r.Chat
	return tea.Batch(s.spinCmd(), func() tea.Msg {
		m, msgs, err := env.Preview(ch)
		m.FirstPrompt, m.LastPrompt = cleanText(m.FirstPrompt), cleanText(m.LastPrompt)
		for i := range msgs {
			msgs[i].Text = cleanText(msgs[i].Text)
		}
		return chatsPreviewMsg{k, chatPreview{m, msgs, err}}
	})
}

// previewPane is the preview of the chat under the cursor, exactly height lines.
func (s *chatsScreen) previewPane(c *Ctx, width, height int) []string {
	var lines []string
	if r, ok := s.current(); ok {
		lines = s.previewLines(c, r, width, height)
	}
	return c.L().Lines(strings.Join(lines, "\n"), width, height)
}

// fullView is the preview filling the body, scrolled in a viewport. Its text keeps to a
// comfortable reading width in a wide window.
func (s *chatsScreen) fullView(c *Ctx, width, height int) string {
	r, ok := s.current()
	if !ok {
		s.full = false
		return ""
	}
	pk := previewKey(r)
	_, done := s.previews[pk]
	key := pk + "|" + strconv.FormatBool(done) + "|" + strconv.Itoa(width)
	if key != s.pagerKey {
		if !strings.HasPrefix(s.pagerKey, pk+"|") {
			s.pager.GotoTop()
		}
		s.pager.SetContentLines(s.previewLines(c, r, min(width, 100), -1))
		s.pagerKey = key
	}
	s.pager.SetWidth(width)
	s.pager.SetHeight(height)
	return s.pager.View()
}

// previewLines draws a chat: what the list knows at once (title, folder, list, dates), then
// what its transcript holds once read (model, counts, first and last prompt, the last
// messages). height < 0 draws everything, for the full-screen preview; otherwise the messages
// shown are the last ones that fit.
func (s *chatsScreen) previewLines(c *Ctx, r chatRow, width, height int) []string {
	l, st := c.L(), c.St
	full, compact := height < 0, height >= 0 && height < 12
	var out []string
	add := func(style lipgloss.Style, lines ...string) {
		for _, t := range lines {
			out = append(out, l.Pad(style.Render(t), width))
		}
	}
	gap := func() {
		if !compact {
			out = append(out, strings.Repeat(" ", width))
		}
	}
	limit := func(n int) int {
		if full {
			return 0
		}
		return n
	}

	title := r.Title
	if title == "" {
		title = i18n.T("chats.untitled")
	}
	titleLines, _ := chatWrap(l, title, width, limit(2)) // a heading keeps to the left edge
	add(st.Title, titleLines...)
	if r.Cwd != "" {
		add(st.Dim, l.Path(r.Cwd, width))
	}
	p, done := s.previews[previewKey(r)]
	add(st.Muted, s.whereLines(c, r, p, width)...)
	switch {
	case !done:
		out = append(out, l.Pad(l.Inline(s.spinner(c), " ", st.Muted.Render(l.Fit(i18n.T("chats.preview.loading"), width-2))), width))
		return out
	case p.err != nil:
		style := st.Fail
		if ops.Is(p.err, ops.NoHistory) {
			style = st.Warn
		}
		add(style, l.Wrap(p.err.Error(), width)...)
		return out
	}
	add(st.Muted, countsLines(l, r, p, width)...)

	block := func(label, text string) {
		if text == "" {
			return
		}
		gap()
		add(st.Muted, l.Fit(label, width))
		out = append(out, para(l, st.Text, text, width, limit(3))...)
	}
	block(i18n.T("chats.preview.first"), p.meta.FirstPrompt)
	if p.meta.LastPrompt != p.meta.FirstPrompt {
		block(i18n.T("chats.preview.last"), p.meta.LastPrompt)
	}
	return append(out, s.recentLines(c, p.msgs, width, height-len(out))...)
}

// whereLines are the chat's list, when it was created and when it was last used.
func (s *chatsScreen) whereLines(c *Ctx, r chatRow, p chatPreview, width int) []string {
	l := c.L()
	parts := []string{l.Data(r.List.Label, width)}
	if r.lost {
		parts[0] = l.Text(i18n.T("chats.chip.lost"))
	}
	created := r.Created
	if created.IsZero() {
		created = p.meta.Start
	}
	if !created.IsZero() {
		parts = append(parts, l.Text(i18n.T("chats.preview.created", "date", i18n.Date(created.Local()))))
	}
	if !r.Last.IsZero() {
		parts = append(parts, l.Text(i18n.T("chats.preview.used", "ago", i18n.Ago(r.Last.Local(), c.Now()))))
	}
	return dotted(l, width, parts...)
}

// countsLines are the models that replied and how many prompts and replies the chat has.
func countsLines(l Layout, r chatRow, p chatPreview, width int) []string {
	var parts []string
	if m := models(p.msgs, cmp.Or(p.meta.Model, r.Model)); m != "" {
		parts = append(parts, l.Path(m, width))
	}
	parts = append(parts, l.Text(i18n.T("chats.preview.prompts", "n", p.meta.Prompts)),
		l.Text(i18n.T("chats.preview.replies", "n", p.meta.Replies)))
	return dotted(l, width, parts...)
}

// models are the models of the messages read, in the order they first replied, or fallback
// when none says (as in exports).
func models(msgs []transcript.Message, fallback string) string {
	var seen []string
	for _, m := range msgs {
		if m.Model != "" && !slices.Contains(seen, m.Model) {
			seen = append(seen, m.Model)
		}
	}
	if len(seen) == 0 {
		return fallback
	}
	return strings.Join(seen, ", ")
}

// dotted joins prepared pieces with " · ", going on to a new line where the next piece would not
// fit in width. Each piece is laid out on its own, so a model name or a list label never pulls
// the numbers next to it along.
func dotted(l Layout, width int, parts ...string) []string {
	var lines []string
	line := ""
	for _, p := range parts {
		switch {
		case line == "":
			line = p
		case ansi.StringWidth(line)+3+ansi.StringWidth(p) <= width:
			line += " · " + p
		default:
			lines = append(lines, l.Clip(line, width))
			line = p
		}
	}
	return append(lines, l.Clip(line, width))
}

// recentLines is a mini transcript: a label, then each message's author, time and text. With
// room >= 0 only the last messages that fit in room lines, three lines of text each.
func (s *chatsScreen) recentLines(c *Ctx, msgs []transcript.Message, width, room int) []string {
	l, st := c.L(), c.St
	full := room < 0
	var blocks [][]string
	used := 2 // the blank line and the label
	for i := len(msgs) - 1; i >= 0; i-- {
		b := s.messageLines(c, msgs[i], width, full)
		if !full && used+len(b) > room {
			break
		}
		used += len(b)
		blocks = append(blocks, b)
	}
	if len(blocks) == 0 {
		return nil
	}
	out := []string{strings.Repeat(" ", width), l.Pad(st.Muted.Render(l.Fit(i18n.T("chats.preview.recent"), width)), width)}
	for i := len(blocks) - 1; i >= 0; i-- {
		out = append(out, blocks[i]...)
	}
	return out
}

// messageLines is one message: who wrote it and when, then its text (or the tools it called).
// Short, it is one paragraph of at most three lines; full, every line of it.
func (s *chatsScreen) messageLines(c *Ctx, m transcript.Message, width int, full bool) []string {
	l, st := c.L(), c.St
	who, style := i18n.T("chats.preview.you"), st.Accent
	if m.Role == "assistant" {
		who, style = i18n.T("chats.preview.claude"), st.Bold
	}
	head := style.Render(l.Text(who))
	if !m.Time.IsZero() {
		head = l.Inline(head, "  ", st.Dim.Render(l.Text(i18n.Ago(m.Time.Local(), c.Now()))))
	}
	out := []string{l.Pad(head, width)}
	text, body := m.Text, st.Text
	if strings.TrimSpace(text) == "" && len(m.Tools) > 0 {
		text, body = i18n.T("chats.preview.tools", "tools", strings.Join(m.Tools, ", ")), st.Dim
	}
	paras, most := []string{text}, 3
	if full {
		paras, most = strings.Split(text, "\n"), 0
	}
	blank := strings.Repeat(" ", width)
	for _, p := range paras {
		if strings.TrimSpace(p) == "" && out[len(out)-1] != blank && len(out) > 1 {
			out = append(out, blank) // a paragraph break
		}
		out = append(out, para(l, body, p, width, most)...)
	}
	return out
}

// cleanText drops control characters a terminal would act on, such as the escape codes of
// pasted terminal output, keeping line breaks.
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}

// para is a paragraph of user text indented two cells, width cells wide in all, lined up at
// the edge it reads from: a Persian prompt starts every line at the right.
func para(l Layout, style lipgloss.Style, text string, width, most int) []string {
	room := width - 2
	lines, dir := chatWrap(l, text, room, most)
	for i, t := range lines {
		pad := ""
		if dir == rtl.RTL {
			pad = strings.Repeat(" ", max(0, room-ansi.StringWidth(t)))
		}
		lines[i] = l.Pad("  "+pad+style.Render(t), width)
	}
	return lines
}

// chatWrap breaks a paragraph of user text into lines of at most width cells and returns its
// direction. Every line takes the paragraph's direction (UAX #9 finds it once, from the first
// strong letter), so a line of a Persian prompt that starts with an English word still reads
// right to left. A word wider than a line, such as a path, is broken. most > 0 keeps that many
// lines, the last cut with "…".
func chatWrap(l Layout, text string, width, most int) ([]string, rtl.Dir) {
	dir := dirOf(text)
	if width <= 0 {
		return nil, dir
	}
	// The preview draws every frame: text past what most lines can hold is never laid out.
	if cut := most * (width + 1) * 8; most > 0 && len(text) > cut {
		for !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case l.Mode.Width(line+" "+word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
		for l.Mode.Width(line) > width {
			head := widthPrefix(l, line, width)
			lines = append(lines, head)
			line = line[len(head):]
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if most > 0 && len(lines) > most {
		lines = append(lines[:most-1], strings.Join(lines[most-1:], " "))
	}
	for i, t := range lines {
		lines[i] = l.Mode.Fit(t, width, dir)
	}
	return lines, dir
}

// widthPrefix is the longest start of s, one character at least, that fits in width cells.
func widthPrefix(l Layout, s string, width int) string {
	end := 0
	for i := range s {
		if l.Mode.Width(s[:i]) > width {
			break
		}
		end = i
	}
	if end == 0 {
		_, end = utf8.DecodeRuneInString(s)
	}
	return s[:end]
}
