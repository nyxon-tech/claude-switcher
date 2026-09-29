package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
)

// Keys are what applies right now: the pager's, the search field's, or the list's, with the
// row actions only when there are rows and copy and move only outside Lost.
func (s *chatsScreen) Keys(c *Ctx) []key.Binding {
	lists := bind("tab", "chats.key.lists", "tab", "shift+tab")
	switch {
	case !s.loaded:
		return nil
	case s.full:
		return []key.Binding{bind("↑↓", "chats.key.scroll", "up", "down", "pgup", "pgdown", "home", "end"), bind("esc", "ui.key.back")}
	case s.typing:
		return []key.Binding{bind("↑↓", "ui.key.move", "up", "down"), bind("enter", "chats.key.done"), bind("esc", "chats.key.clear"), lists}
	}
	keys := []key.Binding{bind("↑↓", "ui.key.move", "up", "down", "pgup", "pgdown", "home", "end")}
	if len(s.shown) > 0 {
		lost := s.chip == chipLost
		if lost {
			keys = append(keys, bind("enter", "chats.key.recover", "enter", "r"))
		} else {
			keys = append(keys, bind("enter", "chats.key.open"))
		}
		sel := bind("space", "chats.key.select")
		if n := len(s.selected); n > 0 {
			sel = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", i18n.T("chats.key.selected", "n", n)))
		}
		keys = append(keys, sel)
		if !lost {
			keys = append(keys, bind("c", "chats.key.copy"), bind("m", "chats.key.move"))
		}
		keys = append(keys, bind("e", "chats.key.export"))
	}
	keys = append(keys, bind("/", "chats.key.search"), lists)
	if len(s.shown) > 0 {
		keys = append(keys, bind("ctrl+a", "chats.key.all"))
	}
	if s.input.Value() != "" || len(s.selected) > 0 {
		keys = append(keys, bind("esc", "chats.key.clear"))
	}
	return keys
}

func (s *chatsScreen) key(c *Ctx, msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case !s.loaded:
		return nil
	case s.full:
		return s.pagerKeys(msg)
	case s.typing:
		return s.typingKey(msg)
	}
	switch k := msg.String(); k {
	case "up", "down", "pgup", "pgdown", "home", "end":
		return s.move(k)
	case "tab":
		return s.nextChip(1)
	case "shift+tab":
		return s.nextChip(-1)
	case "esc":
		return s.clear()
	case "/":
		s.focus()
		return nil
	case "space":
		s.toggle()
		return nil
	case "ctrl+a":
		s.toggleAll()
		return nil
	case "enter":
		return s.activate(c)
	}
	if cmd, ok := s.action(c, msg.String()); ok {
		return cmd
	}
	if printable(msg) { // typing anything else searches for it
		s.focus()
		return s.edit(msg)
	}
	return nil
}

// action runs a letter that acts on chats, when it applies here.
func (s *chatsScreen) action(c *Ctx, k string) (tea.Cmd, bool) {
	rows, lost := len(s.shown) > 0, s.chip == chipLost
	switch {
	case rows && !lost && (k == "c" || k == "m"):
		return s.transfer(c, k == "m"), true
	case rows && k == "e":
		return s.export(c), true
	case rows && lost && k == "r":
		return s.recover(c), true
	}
	return nil, false
}

func printable(msg tea.KeyPressMsg) bool {
	return msg.Text != "" && !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt)
}

// typingKey handles a key while the search field has focus: the list still moves, and enter
// or esc leave the field.
func (s *chatsScreen) typingKey(msg tea.KeyPressMsg) tea.Cmd {
	switch k := msg.String(); k {
	case "up", "down", "pgup", "pgdown":
		return s.move(k)
	case "tab":
		return s.nextChip(1)
	case "shift+tab":
		return s.nextChip(-1)
	case "ctrl+a":
		s.toggleAll()
		return nil
	case "enter":
		s.blur()
		return nil
	case "esc":
		if s.input.Value() == "" {
			s.blur()
			return nil
		}
		s.input.Reset()
		return s.searched()
	}
	return s.edit(msg)
}

// edit passes a key or a paste to the search field and searches again when the text changed.
func (s *chatsScreen) edit(msg tea.Msg) tea.Cmd {
	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() == before {
		return cmd
	}
	return tea.Batch(cmd, s.searched())
}

func (s *chatsScreen) searched() tea.Cmd {
	s.filter()
	s.cursor, s.top = 0, 0
	return s.moved()
}

func (s *chatsScreen) focus() {
	s.typing = true
	s.input.Focus() // no cursor blinks: the field draws its own
}

func (s *chatsScreen) blur() {
	s.typing = false
	s.input.Blur()
}

// clear is esc outside the field: the search goes first, then the selection.
func (s *chatsScreen) clear() tea.Cmd {
	if s.input.Value() != "" {
		s.input.Reset()
		return s.searched()
	}
	clear(s.selected)
	return nil
}

func (s *chatsScreen) move(k string) tea.Cmd {
	n := len(s.shown)
	if n == 0 {
		return nil
	}
	page := max(1, s.geo.rows)
	cur := s.cursor
	switch k {
	case "up":
		cur--
	case "down":
		cur++
	case "pgup":
		cur -= page
	case "pgdown":
		cur += page
	case "home":
		cur = 0
	case "end":
		cur = n - 1
	}
	return s.moveTo(cur)
}

func (s *chatsScreen) moveTo(i int) tea.Cmd {
	i = min(max(i, 0), len(s.shown)-1)
	if i == s.cursor || i < 0 {
		return nil
	}
	s.cursor = i
	return s.moved()
}

// nextChip opens the chip d places on, wrapping around.
func (s *chatsScreen) nextChip(d int) tea.Cmd {
	chips := s.chips()
	i := 0
	for j, ch := range chips {
		if ch.key == s.chip {
			i = j
		}
	}
	n := len(chips)
	return s.setChip(chips[((i+d)%n+n)%n].key)
}

// setChip opens a chip. The search stays; the selection belongs to the chip it was made in.
func (s *chatsScreen) setChip(chip string) tea.Cmd {
	if chip == s.chip {
		return nil
	}
	s.chip = chip
	clear(s.selected)
	return s.searched()
}

func (s *chatsScreen) toggle() {
	r, ok := s.current()
	if !ok {
		return
	}
	if s.selected[r.key] {
		delete(s.selected, r.key)
	} else {
		s.selected[r.key] = true
	}
}

// toggleAll selects every row shown, or clears them when they all are selected already.
func (s *chatsScreen) toggleAll() {
	rows := s.rows()
	all := true
	for _, i := range s.shown {
		all = all && s.selected[rows[i].key]
	}
	for _, i := range s.shown {
		if all {
			delete(s.selected, rows[i].key)
		} else {
			s.selected[rows[i].key] = true
		}
	}
}

// activate is enter on a row: recover a lost chat, or open the preview full-screen.
func (s *chatsScreen) activate(c *Ctx) tea.Cmd {
	if len(s.shown) == 0 {
		return nil
	}
	if s.chip == chipLost {
		return s.recover(c)
	}
	s.full, s.pagerKey = true, ""
	return s.readPreview(c)
}

func (s *chatsScreen) pagerKeys(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		s.full = false
	case "home":
		s.pager.GotoTop()
	case "end":
		s.pager.GotoBottom()
	default:
		s.pager, _ = s.pager.Update(msg)
	}
	return nil
}

// click picks a chip, focuses the search field, or moves to a row; a click on the row under
// the cursor opens it.
func (s *chatsScreen) click(c *Ctx, m tea.MouseClickMsg) tea.Cmd {
	if !s.loaded || s.full || m.Button != tea.MouseLeft {
		return nil
	}
	g := s.geo
	switch {
	case m.Y == chatsChipsY:
		for i, sp := range g.chips {
			if m.X >= sp.x0 && m.X < sp.x1 {
				return s.setChip(g.chipKeys[i])
			}
		}
	case m.Y == chatsSearchY:
		s.focus()
	case m.Y >= chatsListY && m.Y < chatsListY+g.rows && m.X < g.listW:
		i := s.top + m.Y - chatsListY
		if i == s.cursor {
			return s.activate(c)
		}
		return s.moveTo(i)
	}
	return nil
}

func (s *chatsScreen) wheel(m tea.MouseWheelMsg) tea.Cmd {
	if s.full {
		s.pager, _ = s.pager.Update(m)
		return nil
	}
	switch m.Button {
	case tea.MouseWheelUp:
		return s.moveTo(s.cursor - 3)
	case tea.MouseWheelDown:
		return s.moveTo(s.cursor + 3)
	}
	return nil
}
