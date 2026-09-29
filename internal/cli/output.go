package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

// txt readies a line of UI text: user data in it, such as a Persian name, joined and reordered
// where the terminal cannot.
func (a *app) txt(s string) string { return a.mode.Line(s, rtl.LTR) }

// para readies several lines of UI text, wrapped to the window.
func (a *app) para(s string, width int) string {
	lines := a.wrap(s, width)
	for i, l := range lines {
		lines[i] = a.txt(l)
	}
	return strings.Join(lines, "\n")
}

// wrap breaks logical text into lines of at most width cells, before any reordering: a terminal
// wrapping a reordered right-to-left line would show its end first. Piped output keeps its lines.
func (a *app) wrap(s string, width int) []string {
	if a.tty {
		s = ansi.Wrap(s, max(width, 20), "")
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return lines
}

// fit readies user data (titles, folders, names) cut to width cells; its direction comes from
// its first letter.
func (a *app) fit(s string, width int) string { return a.mode.Fit(s, width, rtl.DirAuto) }

// line joins prepared parts with spaces.
func (a *app) line(parts ...string) string { return strings.Join(parts, " ") }

// print writes a line or a block.
func (a *app) print(w io.Writer, s string) { fmt.Fprintln(w, s) }

// say writes msg after lead (a rendered mark such as ✓, or spaces), wrapped to the window;
// wrapped lines line up under the first.
func (a *app) say(w io.Writer, lead string, style lipgloss.Style, msg string) {
	pad := strings.Repeat(" ", lipgloss.Width(lead))
	for i, l := range a.wrap(msg, a.width-2-len(pad)) {
		if i > 0 {
			lead = pad
		}
		a.print(w, a.line(lead, style.Render(a.txt(l))))
	}
}

func (a *app) done(w io.Writer, msg string) { a.say(w, a.st.ok.Render("✓"), a.st.plain, msg) }
func (a *app) warn(w io.Writer, msg string) { a.say(w, a.st.warn.Render("!"), a.st.plain, msg) }
func (a *app) note(w io.Writer, msg string) { a.say(w, " ", a.st.muted, msg) }

// heading writes a section title after a blank line.
func (a *app) heading(title string) {
	fmt.Fprintln(a.out)
	a.print(a.out, a.st.bold.Render(a.txt(title)))
}

// table draws rows of prepared cells under headers (plain UI text): rounded borders in the line
// colour, muted headers, the numeric columns right-aligned.
func (a *app) table(headers []string, rows [][]string, numeric ...int) string {
	for i := range headers {
		headers[i] = a.txt(headers[i])
	}
	return table.New().Border(lipgloss.RoundedBorder()).BorderStyle(a.st.border).
		Headers(headers...).Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				s = s.Inherit(a.st.muted)
			}
			if slices.Contains(numeric, col) {
				s = s.Align(lipgloss.Right)
			}
			return s
		}).Render()
}

// fitColumn fills column col of every row with fill(i, width), width being what the window
// leaves once the other columns and the borders have their room, so the table never overflows.
func (a *app) fitColumn(headers []string, rows [][]string, col int, fill func(i, width int) string) {
	used := 3*len(headers) + 2 // padding and borders, and the last column of the window left free
	for c, h := range headers {
		if c == col {
			continue
		}
		w := lipgloss.Width(a.txt(h))
		for _, r := range rows {
			w = max(w, lipgloss.Width(r[c]))
		}
		used += w
	}
	width := max(a.width-used, lipgloss.Width(a.txt(headers[col])), 8)
	for i := range rows {
		rows[i][col] = fill(i, width)
	}
}

// pairs writes labels (plain UI text) and their prepared values, two cells indented, the values
// lined up two cells after the longest label.
func (a *app) pairs(rows [][2]string) {
	width := 0
	for _, r := range rows {
		width = max(width, lipgloss.Width(a.txt(r[0])))
	}
	label := lipgloss.NewStyle().Width(width + 1).Inherit(a.st.muted)
	for _, r := range rows {
		if r[1] != "" {
			a.print(a.out, a.line(" ", label.Render(a.txt(r[0])), r[1]))
		}
	}
}

// json writes v as indented UTF-8 JSON, with <, > and & left as they are.
func (a *app) json(v any) error {
	enc := json.NewEncoder(a.stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// confirm asks a yes-or-no question on stderr. --yes answers it; without a terminal to ask on,
// it refuses instead of waiting for an answer that never comes.
func (a *app) confirm(ctx context.Context, question string) error {
	if a.flags.yes {
		return nil
	}
	if !a.ttyIn {
		return &cliError{err: errText("cli.confirm.refused", "question", question), code: exitRefused, hint: i18n.T("cli.hint.yes")}
	}
	mark, keys := a.st.accent.Render("?"), a.st.muted.Render(i18n.T("cli.confirm.keys"))
	prompt := a.line(mark, a.txt(question), keys)
	if lipgloss.Width(prompt) > a.width-8 { // leave room for the answer
		a.say(a.err, mark, a.st.plain, question)
		prompt = keys
	}
	fmt.Fprint(a.err, prompt+" ")
	answer := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(a.in).ReadString('\n')
		answer <- s
	}()
	select {
	case <-ctx.Done():
		fmt.Fprintln(a.err)
		return cancelled()
	case s := <-answer:
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "", "y", "yes":
			return nil
		}
		return cancelled()
	}
}

// spin runs fn while a spinner turns next to label on stderr, when stderr is a terminal.
func (a *app) spin(ctx context.Context, label func() string, fn func(context.Context) error) error {
	if !a.ttyErr {
		return fn(ctx)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		s := spinner.MiniDot
		tick := time.NewTicker(s.FPS)
		defer tick.Stop()
		for i := 0; ; i++ {
			frame := a.st.accent.Render(s.Frames[i%len(s.Frames)])
			fmt.Fprint(a.err, "\r"+ansi.EraseEntireLine+a.line(frame, a.txt(label())))
			select {
			case <-stop:
				fmt.Fprint(a.err, "\r"+ansi.EraseEntireLine)
				return
			case <-tick.C:
			}
		}
	})
	err := fn(ctx)
	close(stop)
	wg.Wait()
	return err
}
