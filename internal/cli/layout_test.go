package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// TestLayoutFitsTheWindow checks what a terminal shows: no line is wider than the window (a
// terminal wrapping a reordered Persian line would show its end first), and mirrored Persian
// output is flush with the right edge.
func TestLayoutFitsTheWindow(t *testing.T) {
	f := chatFixture(t)
	f.transcript(o1, persianTitle+" ۲ (نسخه‌ی ۳) و یک عنوان بلند که ادامه پیدا می‌کند", "fix", time.Now().Add(-48*time.Hour))
	// A list no profile owns ("account 33333333") and a chat with a long title in a long folder.
	long := filepath.Join(f.data, "claude-code-sessions", "33333333-3333-4333-8333-333333333333", org, "local_c4.json")
	f.put(long, `{"sessionId":"local_c4","cliSessionId":"10000000-0000-4000-8000-000000000004","cwd":"/work/a-very-long-project-folder-name",`+
		`"title":"A much longer English title that goes on and on to test the title column","createdAt":1,"lastActivityAt":2}`)
	f.ok("copy", "--from", "work", "--to", "personal", "c1", "--yes")

	commands := [][]string{{"list"}, {"chats"}, {"chats", "--lost"}, {"history"}, {"doctor"}, {"usage", "--days", "365"},
		{"config"}, {"version"}, {"switch", "nobody"}, {"copy", "--from", "work", "--to", "personal", "c2"}}
	for _, lang := range []string{"en", "fa"} {
		for _, width := range []int{60, 80, 140} {
			for _, args := range commands {
				r := f.term(width, append(args, "--lang", lang)...)
				for _, l := range strings.Split(strings.TrimRight(r.out+r.err, "\n"), "\n") {
					w := ansi.StringWidth(l)
					if w > width-1 || lang == "fa" && l != "" && w != width-1 {
						t.Errorf("%s at %d columns, %v: a line %d wide:\n%s", lang, width, args, w, r.out+r.err)
						break
					}
				}
			}
		}
	}
}

// TestPersianFlagsKeepTheirDashes: laid out right to left, "--yes" would read "yes--".
func TestPersianFlagsKeepTheirDashes(t *testing.T) {
	f := chatFixture(t)
	r := f.term(80, "remove", "personal", "--lang", "fa")
	if !strings.Contains(r.err, "--yes") || strings.Contains(r.err, "yes--") {
		t.Errorf("the hint should name --yes as it is typed:\n%s", r.err)
	}
}

// TestLongMessagesWrap: messages wrap at the window, continuation lines under the text.
func TestLongMessagesWrap(t *testing.T) {
	f := chatFixture(t)
	r := f.term(40, "remove", "personal")
	lines := strings.Split(strings.TrimRight(r.err, "\n"), "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "✗ ") || !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("a long error should wrap under its text:\n%s", r.err)
	}
}
