package ui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

// picked are the chats an action works on: the chip's selected rows, else the one under the
// cursor.
func (s *chatsScreen) picked() []chatRow {
	var out []chatRow
	for _, r := range s.rows() {
		if s.selected[r.key] {
			out = append(out, r)
		}
	}
	if r, ok := s.current(); ok && len(out) == 0 {
		out = append(out, r)
	}
	return out
}

// transfer copies or moves the picked chats: pick a list, see the plan, confirm, then the
// runner writes it with Desktop closed.
func (s *chatsScreen) transfer(c *Ctx, move bool) tea.Cmd {
	rows := s.picked()
	from := rows[0].List
	sources := map[string]bool{}
	ids := make([]string, len(rows))
	for i, r := range rows {
		sources[r.List.Dir] = true
		ids[i] = r.ID
	}
	if len(sources) > 1 {
		c.Alert(i18n.T("chats.mixed.title"), i18n.T("chats.mixed.body", "n", len(sources)))
		return nil
	}
	var targets []ops.List
	for _, l := range s.lists {
		if l.Dir != from.Dir {
			targets = append(targets, l)
		}
	}
	if len(targets) == 0 {
		c.Alert(i18n.T("chats.no_target.title"), i18n.T("chats.no_target.body"))
		return nil
	}
	pick := "chats.copy.pick"
	if move {
		pick = "chats.move.pick"
	}
	env := c.Env
	chooseList(c, i18n.T(pick, "n", len(rows)), targets, firstFull(targets), func(to ops.List) tea.Cmd {
		return func() tea.Msg {
			plan, err := env.PlanTransfer(from, to, ids, move)
			return chatsPlanMsg{plan, err}
		}
	})
	return nil
}

// firstFull pre-selects the signed-in list, else the first list with chats; an empty list is
// never pre-selected (-1), so it is only picked on purpose.
func firstFull(lists []ops.List) int {
	pick := -1
	for i, l := range lists {
		switch {
		case l.Chats == 0:
		case l.SignedIn:
			return i
		case pick < 0:
			pick = i
		}
	}
	return pick
}

// chooseList asks for a list, showing how many chats each holds. Enter with nothing
// highlighted asks again.
func chooseList(c *Ctx, title string, lists []ops.List, cursor int, then func(ops.List) tea.Cmd) {
	options := make([]string, len(lists))
	for i, l := range lists {
		options[i] = i18n.T("chats.target", "label", l.Label, "chats", i18n.T("count.chats", "n", l.Chats))
	}
	c.Choose(title, "", options, func(i int) tea.Cmd {
		if i < 0 {
			chooseList(c, title, lists, cursor, then)
			return nil
		}
		return then(lists[i])
	})
	c.dialog.cursor = cursor
}

// confirmTransfer shows the plan's counts and runs it once confirmed.
func (s *chatsScreen) confirmTransfer(c *Ctx, msg chatsPlanMsg) tea.Cmd {
	if c.runner != nil || c.dialog != nil { // the user moved on meanwhile
		return nil
	}
	if msg.err != nil {
		return c.Toast(toastFail, msg.err.Error())
	}
	p := msg.plan
	n, to := len(p.Steps), p.To.Label
	add, upd := p.Count(ops.Create), p.Count(ops.Update)
	kind := "copy"
	if p.Kind == ops.Move {
		kind = "move"
	}
	title := i18n.T("chats."+kind+".title", "n", n, "to", to)
	if kind == "copy" && add+upd == 0 {
		c.Alert(title, i18n.T("chats.copy.nothing", "to", to))
		return nil
	}
	body := i18n.T("chats.plan.counts", "new", add, "updated", upd, "newer", p.Count(ops.SkipNewer)) + "\n" +
		i18n.T("chats."+kind+".body", "from", p.From.Label, "to", to)
	env, undo := c.Env, i18n.T("chats.undo_hint")
	c.Confirm(title, body, i18n.T("action."+kind), false, func() tea.Cmd {
		return s.change(c, change{
			title: i18n.T("chats."+kind+".running", "n", n, "to", to),
			steps: []string{i18n.T("chats.step."+kind, "to", to)},
			run: func() (string, error) {
				r, err := env.Apply(p)
				return r.Summary + " · " + undo, err
			},
		})
	})
	return nil
}

// recover gives the picked lost chats a card in a list (the signed-in one unless the user
// picks another), so Desktop shows them again.
func (s *chatsScreen) recover(c *Ctx) tea.Cmd {
	rows := s.picked()
	if len(s.lists) == 0 {
		c.Alert(i18n.T("chats.no_list.title"), i18n.T("chats.no_target.body"))
		return nil
	}
	sessions := make([]string, len(rows))
	for i, r := range rows {
		sessions[i] = r.Session
	}
	cursor := firstFull(s.lists)
	for i, l := range s.lists {
		if l.SignedIn {
			cursor = i
		}
	}
	n, env, undo := len(rows), c.Env, i18n.T("chats.undo_hint")
	chooseList(c, i18n.T("chats.rescue.pick", "n", n), s.lists, cursor, func(to ops.List) tea.Cmd {
		c.Confirm(i18n.T("chats.rescue.title", "n", n, "to", to.Label), i18n.T("chats.rescue.body", "to", to.Label),
			i18n.T("action.recover"), false, func() tea.Cmd {
				return s.change(c, change{
					title: i18n.T("chats.rescue.running", "n", n, "to", to.Label),
					steps: []string{i18n.T("chats.step.rescue", "to", to.Label)},
					run: func() (string, error) {
						r, err := env.Rescue(to, sessions)
						return r.Summary + " · " + undo, err
					},
				})
			})
		return nil
	})
	return nil
}

// change starts a write; once it is done (or given up) the lists are read again.
func (s *chatsScreen) change(c *Ctx, ch change) tea.Cmd {
	clear(s.selected)
	s.stale = true
	return c.Change(ch)
}

var exportFormats = []string{"html", "md"}

// export writes the picked chats as HTML or Markdown files in the Downloads folder. It only
// reads Claude's data, so it needs no runner.
func (s *chatsScreen) export(c *Ctx) tea.Cmd {
	rows := s.picked()
	env, dir := c.Env, s.downloads
	c.Choose(i18n.T("chats.export.pick", "n", len(rows)), i18n.T("chats.export.body"),
		[]string{i18n.T("chats.export.html"), i18n.T("chats.export.md")}, func(i int) tea.Cmd {
			format := exportFormats[i]
			return func() tea.Msg {
				to := dir
				if to == "" {
					to = downloadsDir()
				}
				var paths []string
				for _, r := range rows {
					path, err := exportChat(env, r.Chat, format, to)
					if err != nil {
						return chatsExportMsg{paths, err}
					}
					paths = append(paths, path)
				}
				return chatsExportMsg{paths: paths}
			}
		})
	return nil
}

func (s *chatsScreen) exported(c *Ctx, msg chatsExportMsg) tea.Cmd {
	switch {
	case msg.err != nil:
		return c.Toast(toastFail, msg.err.Error())
	case len(msg.paths) == 1:
		return c.Toast(toastOK, i18n.T("chats.export.done", "path", msg.paths[0]))
	}
	return c.Toast(toastOK, i18n.T("chats.export.done_many", "n", len(msg.paths), "dir", filepath.Dir(msg.paths[0])))
}

// exportChat writes one chat to a new file in dir, named after its title.
func exportChat(env *ops.Env, ch ops.Chat, format, dir string) (string, error) {
	f, err := createNew(dir, exportName(ch.Title), "."+format)
	if err != nil {
		return "", err
	}
	err = env.Export(ch, format, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name()) // the file was made just now, for this export
		return "", err
	}
	return f.Name(), nil
}

// createNew creates name+ext in dir, or name-2+ext, name-3+ext... when it exists already.
func createNew(dir, name, ext string) (*os.File, error) {
	for n := 1; ; n++ {
		file := name + ext
		if n > 1 {
			file = name + "-" + strconv.Itoa(n) + ext
		}
		f, err := os.OpenFile(filepath.Join(dir, file), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if !errors.Is(err, fs.ErrExist) {
			return f, err
		}
	}
}

// exportName makes a chat title a file name that every OS accepts: no path separators or
// characters Windows forbids, no trailing dot or space, and never a Windows device name.
func exportName(title string) string {
	name := strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`<>:"/\|?*`, r) {
			return ' '
		}
		return r
	}, title)
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > 80 {
		name = string([]rune(name)[:80])
	}
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "chat"
	}
	// Windows reserves CON, NUL, COM1... whatever the extension: "CON.notes.html" is a device.
	base, rest, _ := strings.Cut(name, ".")
	if reservedName(strings.TrimSpace(base)) {
		name = base + "_"
		if rest != "" {
			name += "." + rest
		}
	}
	return name
}

func reservedName(s string) bool {
	s = strings.ToUpper(s)
	switch s {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	r := []rune(s)
	return len(r) == 4 && (strings.HasPrefix(s, "COM") || strings.HasPrefix(s, "LPT")) &&
		strings.ContainsRune("0123456789¹²³", r[3])
}

// downloadsDir is the user's Downloads folder, or the current folder when there is none.
func downloadsDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, "Downloads")
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
