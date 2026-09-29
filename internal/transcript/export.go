package transcript

import (
	"cmp"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"slices"
	"strings"
	"time"
	"unicode"
)

const untitled = "Untitled chat"

// WriteMarkdown writes a chat as Markdown. Raw HTML in the chat is escaped outside code, so
// it shows as text instead of running in a viewer.
func WriteMarkdown(w io.Writer, m Meta, msgs []Message) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", mdLine(cmp.Or(m.Title, untitled)))
	if m.Cwd != "" {
		fmt.Fprintf(&b, "- Project: %s\n", mdLine(m.Cwd))
	}
	if span := dateSpan(m.Start, m.End); span != "" {
		fmt.Fprintf(&b, "- Date: %s\n", span)
	}
	if models := modelsOf(m, msgs); len(models) > 0 {
		fmt.Fprintf(&b, "- Models: %s\n", mdLine(strings.Join(models, ", ")))
	}
	b.WriteString("\n---\n")
	for _, msg := range msgs {
		fmt.Fprintf(&b, "\n### %s", speaker(msg.Role))
		if !msg.Time.IsZero() {
			fmt.Fprintf(&b, " · %s", when(msg.Time))
		}
		b.WriteString("\n\n")
		if msg.Text != "" {
			b.WriteString(mdEscape(msg.Text) + "\n")
		}
		if len(msg.Tools) > 0 {
			fmt.Fprintf(&b, "\n*Tools: `%s`*\n", strings.Join(msg.Tools, "`, `"))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

//go:embed page.html
var pageHTML string

var page = template.Must(template.New("page").Funcs(template.FuncMap{"when": when, "speaker": speaker}).Parse(pageHTML))

// WriteHTML writes a chat as one self-contained HTML page (right-to-left safe: dir="auto").
func WriteHTML(w io.Writer, m Meta, msgs []Message) error {
	title := cmp.Or(m.Title, untitled)
	return page.Execute(w, map[string]any{
		"Lang":   langOf(title),
		"Title":  title,
		"Cwd":    m.Cwd,
		"Span":   dateSpan(m.Start, m.End),
		"Models": strings.Join(modelsOf(m, msgs), ", "),
		"Msgs":   msgs,
	})
}

func speaker(role string) string {
	if role == "user" {
		return "You"
	}
	return "Claude"
}

func when(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

func dateSpan(start, end time.Time) string {
	switch {
	case start.IsZero():
		return ""
	case end.IsZero() || when(start) == when(end):
		return when(start)
	}
	return when(start) + " – " + when(end)
}

// modelsOf lists the models that answered, in order of first reply.
func modelsOf(m Meta, msgs []Message) []string {
	var models []string
	for _, msg := range msgs {
		if msg.Model != "" && !slices.Contains(models, msg.Model) {
			models = append(models, msg.Model)
		}
	}
	if len(models) == 0 && m.Model != "" {
		models = append(models, m.Model)
	}
	return models
}

// langOf guesses the page language from the title's script: Persian for Arabic script.
func langOf(s string) string {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) {
			return "fa"
		}
	}
	return "en"
}

// mdLine puts s on one line with "<" escaped, for the header.
func mdLine(s string) string {
	out, _ := escapeInline(strings.Join(strings.Fields(s), " "), false)
	return out
}

// mdEscape escapes "<" outside code so raw HTML in a chat shows as text in a Markdown viewer.
// Fences follow CommonMark. Where a viewer could read code differently (a backtick left open, an
// indented fence that a list item may end), "<" is escaped rather than trusted. A fence left
// open is closed so it cannot swallow the rest of the export.
func mdEscape(s string) string {
	var b strings.Builder
	fence, indent := "", 0 // the open fence's marker and indent
	strict := false        // escape every "<" until the paragraph ends
	lost := false          // lost track of fences: escape every "<" from here on
	for i, ln := range strings.Split(s, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		ind := len(ln) - len(strings.TrimLeft(ln, " "))
		marker, info := fenceMarker(ln[ind:])
		blank := strings.TrimSpace(ln) == ""
		switch {
		case lost:
			b.WriteString(escapeAll(ln))
		case fence != "":
			closing := strings.HasPrefix(marker, fence) && strings.TrimSpace(info) == ""
			switch {
			case closing && ind >= indent && ind <= 3:
				fence = ""
			case indent > 0 && !blank && (closing || ind < indent):
				fence, lost = "", true
				ln = escapeAll(ln)
			}
			b.WriteString(ln)
		case ind <= 3 && marker != "" && !(marker[0] == '`' && strings.Contains(info, "`")):
			fence, indent, strict = marker, ind, false
			b.WriteString(ln)
		default:
			if blank {
				strict = false
			}
			var out string
			out, strict = escapeInline(ln, strict)
			b.WriteString(out)
		}
	}
	if fence != "" {
		b.WriteString("\n" + strings.Repeat(" ", indent) + fence)
	}
	return b.String()
}

// fenceMarker splits a line without its indent into a leading run of three or more backticks
// or tildes and the rest; marker is "" when there is no such run.
func fenceMarker(t string) (marker, info string) {
	if t == "" || (t[0] != '`' && t[0] != '~') {
		return "", ""
	}
	n := run(t, t[0])
	if n < 3 {
		return "", ""
	}
	return t[:n], t[n:]
}

// escapeInline escapes "<" in one line, leaving `code spans` as they are. A backtick run with no
// closer on the line may pair with one on a later line, so from there on it escapes everything
// and reports strict for the rest of the paragraph.
func escapeInline(s string, strict bool) (string, bool) {
	if strict {
		return escapeAll(s), true
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		switch s[i] {
		case '\\': // an escaped backtick opens no code span
			j := min(i+2, len(s))
			b.WriteString(s[i:j])
			i = j
		case '`':
			n := run(s[i:], '`')
			end := closingBackticks(s[i+n:], n)
			if end < 0 {
				b.WriteString(escapeAll(s[i:]))
				return b.String(), true
			}
			j := i + n + end + n
			b.WriteString(s[i:j])
			i = j
		case '<':
			b.WriteString("&lt;")
			i++
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String(), false
}

func escapeAll(s string) string { return strings.ReplaceAll(s, "<", "&lt;") }

// run counts the bytes c at the start of s.
func run(s string, c byte) int { return len(s) - len(strings.TrimLeft(s, string(c))) }

// closingBackticks returns where the next run of exactly n backticks starts in s, or -1.
func closingBackticks(s string, n int) int {
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		m := run(s[i:], '`')
		if m == n {
			return i
		}
		i += m
	}
	return -1
}
