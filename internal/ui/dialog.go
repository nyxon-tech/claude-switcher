package ui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
)

// dialog is the one modal box: a confirmation, an alert, a text field or a choice list. Buttons
// run left to right and the last one is the primary. enter takes the focused button (or submits
// the field, or picks the choice), esc cancels, y and n answer confirmations, and ←/→ or tab
// move between buttons.
type dialog struct {
	title, body string
	buttons     []string
	focus       int
	danger      bool // the primary button does something that cannot be taken back

	input    *textinput.Model
	validate func(string) string // the problem with a value, "" when it is fine
	problem  string

	choices []string
	cursor  int

	ok func(d *dialog) tea.Cmd // the primary action; nil for alerts
}

// Confirm asks before doing something. Each line of body is a paragraph. yes labels the primary
// button; danger paints it red.
func (c *Ctx) Confirm(title, body, yes string, danger bool, onYes func() tea.Cmd) {
	c.dialog = &dialog{
		title: title, body: body, danger: danger,
		buttons: []string{i18n.T("action.cancel"), yes}, focus: 1,
		ok: func(*dialog) tea.Cmd { return onYes() },
	}
	if danger {
		c.dialog.focus = 0 // a slip of enter should not remove anything
	}
}

// Alert tells the user something that needs no answer.
func (c *Ctx) Alert(title, body string) {
	c.dialog = &dialog{title: title, body: body, buttons: []string{i18n.T("action.close")}}
}

// Prompt asks for a line of text. validate may be nil; submit runs with the value once it passes.
func (c *Ctx) Prompt(title, body, value, placeholder, yes string, validate func(string) string, submit func(string) tea.Cmd) tea.Cmd {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = placeholder
	in.CharLimit = 40
	in.SetStyles(c.St.input())
	in.SetValue(value)
	in.CursorEnd()
	c.dialog = &dialog{
		title: title, body: body, buttons: []string{i18n.T("action.cancel"), yes}, focus: 1,
		input: &in, validate: validate,
		ok: func(d *dialog) tea.Cmd { return submit(strings.TrimSpace(d.input.Value())) },
	}
	return in.Focus()
}

// Choose asks the user to pick one of several options.
func (c *Ctx) Choose(title, body string, options []string, pick func(int) tea.Cmd) {
	c.dialog = &dialog{
		title: title, body: body, choices: options,
		ok: func(d *dialog) tea.Cmd { return pick(d.cursor) },
	}
}

// update handles a key while the dialog is open. It closes the dialog itself through c.
func (d *dialog) update(c *Ctx, msg tea.KeyPressMsg) tea.Cmd {
	switch k := msg.String(); {
	case k == "esc":
		c.dialog = nil
	case k == "enter":
		return d.accept(c)
	case d.input != nil:
		in, cmd := d.input.Update(msg)
		d.input, d.problem = &in, ""
		return cmd
	case len(d.choices) > 0:
		switch k {
		case "up", "k":
			d.cursor = max(0, d.cursor-1)
		case "down", "j":
			d.cursor = min(len(d.choices)-1, d.cursor+1)
		}
	case k == "y" && d.ok != nil:
		c.dialog = nil
		return d.ok(d)
	case k == "n" && d.ok != nil:
		c.dialog = nil
	case k == "tab" || k == "right":
		d.focus = min(d.focus+1, len(d.buttons)-1)
	case k == "shift+tab" || k == "left":
		d.focus = max(d.focus-1, 0)
	}
	return nil
}

// accept is enter: the focused button, the submitted field or the picked choice.
func (d *dialog) accept(c *Ctx) tea.Cmd {
	if d.input != nil && d.validate != nil {
		if d.problem = d.validate(strings.TrimSpace(d.input.Value())); d.problem != "" {
			return nil
		}
	}
	c.dialog = nil
	if d.ok == nil || len(d.buttons) > 0 && d.focus != len(d.buttons)-1 {
		return nil
	}
	return d.ok(d)
}

// keys are the hints under the box.
func (d *dialog) keys() []key.Binding {
	if len(d.choices) > 0 {
		return []key.Binding{bind("↑↓", "ui.key.move"), bind("enter", "ui.key.choose"), bind("esc", "ui.key.cancel")}
	}
	return nil
}

// view draws the box, at most width by height cells.
func (d *dialog) view(c *Ctx, width, height int) string {
	l, st := c.L(), c.St
	inner := boxInner(width)
	var lines []string
	add := func(s ...string) { lines = append(lines, s...) }
	add(l.Pad(st.Title.Render(l.Fit(d.title, inner)), inner))
	for _, para := range strings.Split(d.body, "\n") {
		if para == "" {
			continue
		}
		add("")
		for _, t := range l.Wrap(para, inner) {
			add(l.Pad(st.Text.Render(t), inner))
		}
	}
	if d.input != nil {
		d.input.SetWidth(inner - 5)
		add("")
		field := st.Box(true).Padding(0, 1).Render(d.input.View())
		for _, t := range strings.Split(field, "\n") {
			add(l.Pad(t, inner))
		}
		if d.problem != "" {
			for _, t := range l.Wrap(d.problem, inner) {
				add(l.Pad(st.Fail.Render(t), inner))
			}
		}
	}
	if len(d.choices) > 0 {
		add("")
		for i, ch := range d.choices {
			add(choiceRow(c, ch, i == d.cursor, inner))
		}
	}
	if len(d.buttons) > 0 {
		add("", l.End(d.buttonRow(c), inner))
	} else if ks := d.keys(); len(ks) > 0 {
		add("", hintLine(c, ks, inner))
	}
	return boxed(st, lines, height)
}

func (d *dialog) buttonRow(c *Ctx) string {
	l, st := c.L(), c.St
	labels := make([]string, len(d.buttons))
	for i, b := range d.buttons {
		label := " " + l.Text(b) + " "
		switch {
		case i != d.focus:
			label = st.Muted.Render(label)
		case d.danger && i == len(d.buttons)-1:
			label = st.DangerBadge.Render(label)
		default:
			label = st.Badge.Render(label)
		}
		labels[i] = label
	}
	return strings.Join(labels, "  ")
}

// choiceRow is one option of a list, with the selection bar on the focused one.
func choiceRow(c *Ctx, text string, focused bool, width int) string {
	l, st := c.L(), c.St
	text = l.Fit(text, width-2) // options are interface text, such as list labels
	if !focused {
		return l.Pad("  "+st.Text.Render(text), width)
	}
	return l.Pad(st.Bar.Render("▌")+" "+st.Accent.Render(text), width)
}

// boxInner is the text width of a dialog or runner box in a window width cells wide: at most 52,
// leaving room for the border, two cells of padding and two of margin on each side.
func boxInner(width int) int { return min(52, width-10) }

// boxed draws a dialog or runner box around lines ("" is a blank line). When it would be taller
// than height, the blank lines and the padding go first, so the buttons stay on screen.
func boxed(st Styles, lines []string, height int) string {
	pad := 1
	if len(lines)+4 > height {
		pad, lines = 0, slices.DeleteFunc(slices.Clone(lines), func(s string) bool { return s == "" })
	}
	return st.Box(true).Padding(pad, 2).Render(strings.Join(lines, "\n"))
}

// place returns the top-left cell that centres a box in an area.
func place(box string, width, height int) (x, y int) {
	w, h := ansi.StringWidth(strings.SplitN(box, "\n", 2)[0]), strings.Count(box, "\n")+1
	return max(0, (width-w)/2), max(0, (height-h)/2)
}
