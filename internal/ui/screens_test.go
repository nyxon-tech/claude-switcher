package ui

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/uax29/v2/graphemes"

	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

// TestDocScreens renders the README screenshots from the real app on the made-up setup:
//
//	go run ./tools/demodata -out demo
//	CLAUDE_SWITCHER_DEMO=demo CLAUDE_SWITCHER_SCREENS=out go test ./internal/ui -run TestDocScreens
//
// Each screen becomes an HTML page drawn cell by cell like a terminal; print it to PNG with any
// browser. It is skipped unless both variables are set.
func TestDocScreens(t *testing.T) {
	demo, out := os.Getenv("CLAUDE_SWITCHER_DEMO"), os.Getenv("CLAUDE_SWITCHER_SCREENS")
	if demo == "" || out == "" {
		t.Skip("set CLAUDE_SWITCHER_DEMO and CLAUDE_SWITCHER_SCREENS to render the README screenshots")
	}
	t.Setenv("NO_COLOR", "")
	for _, s := range []struct {
		name          string
		tab           int
		width, height int
	}{{"hero", 0, 112, 30}, {"chats", 1, 112, 30}, {"usage", 3, 112, 26}} {
		env, err := ops.Open(ops.Options{DataDir: filepath.Join(demo, "claude"), VaultDir: filepath.Join(demo, "vault"), ProjectsDir: filepath.Join(demo, "projects")})
		if err != nil {
			t.Fatal(err)
		}
		env.Desktop = &fakeDesktop{}
		a := newApp(env, Options{Version: "3.0.0", Mode: rtl.App, Theme: "dark"})
		a.Update(tea.WindowSizeMsg{Width: s.width, Height: s.height})
		settle(a, a.Init())
		settle(a, a.show(s.tab))
		settle(a, nil) // results that arrive late, such as a debounced preview
		page := terminalPage(a.View().Content)
		if err := os.WriteFile(filepath.Join(out, s.name+".html"), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// terminalPage draws a frame of the app as an HTML page that looks like a terminal window.
func terminalPage(frame string) string {
	var rows strings.Builder
	for _, line := range strings.Split(frame, "\n") {
		rows.WriteString(`<div class="row">`)
		rows.WriteString(cellsHTML(line))
		rows.WriteString("</div>\n")
	}
	return fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><style>
body { margin: 0; background: transparent; }
.win { display: inline-block; margin: 56px; border-radius: 14px; background: #0b0e1a; border: 1px solid #303345;
  box-shadow: 0 16px 44px rgba(9, 11, 20, .45); overflow: hidden; }
.bar { height: 38px; display: flex; align-items: center; padding: 0 16px; gap: 8px; border-bottom: 1px solid #1a1d2d; }
.dot { width: 12px; height: 12px; border-radius: 50%%; background: #303345; }
.title { flex: 1; text-align: center; color: #6b7090; font: 13px 'Segoe UI', system-ui, sans-serif; margin-right: 52px; }
.term { padding: 16px 20px 18px; font-family: 'Cascadia Mono', 'Cascadia Code', Consolas, monospace; font-size: 15px;
  color: #e8e6df; }
.row { display: flex; height: 19px; line-height: 19px; }
.c { display: inline-block; width: 1ch; flex: none; text-align: center; white-space: pre; }
.w { width: 2ch; }
</style></head><body><div class="win"><div class="bar"><span class="dot"></span><span class="dot"></span><span class="dot"></span><span class="title">claude-switcher</span></div>
<div class="term">
%s</div></div></body></html>
`, rows.String())
}

type cellStyle struct {
	fg, bg                                  string
	bold, faint, italic, under, rev, strike bool
}

// cellsHTML turns one line of text with SGR escape codes into one span per terminal cell.
func cellsHTML(line string) string {
	var b strings.Builder
	var st cellStyle
	for len(line) > 0 {
		if strings.HasPrefix(line, "\x1b[") {
			end := strings.IndexAny(line[2:], "@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~")
			if end < 0 {
				break
			}
			if line[2+end] == 'm' {
				st = st.apply(line[2 : 2+end])
			}
			line = line[3+end:]
			continue
		}
		if strings.HasPrefix(line, "\x1b]") { // OSC, ended by BEL or ST
			if i := strings.IndexAny(line, "\a\x9c"); i >= 0 {
				line = line[i+1:]
			} else if i := strings.Index(line, "\x1b\\"); i >= 0 {
				line = line[i+2:]
			} else {
				break
			}
			continue
		}
		g, w := firstCluster(line)
		line = line[len(g):]
		if w == 0 {
			continue
		}
		class := "c"
		if w == 2 {
			class = "c w"
		}
		if fill := blockFill(g, st); fill != "" {
			b.WriteString(`<span class="c" style="background:` + fill + `"></span>`)
			continue
		}
		b.WriteString(`<span class="` + class + `"` + st.css() + `>` + html.EscapeString(g) + `</span>`)
	}
	return b.String()
}

// firstCluster is the first grapheme cluster of s and how many cells it takes.
func firstCluster(s string) (string, int) {
	g := graphemes.FromString(s)
	g.Next()
	return g.Value(), ansi.StringWidth(g.Value())
}

// blockFill paints block elements (bars and charts) as a fill of the whole cell, the way
// terminals draw them; font glyphs would leave seams between cells. "" for any other text.
func blockFill(g string, st cellStyle) string {
	rs := []rune(g)
	if len(rs) != 1 || rs[0] < 0x2580 || rs[0] > 0x2593 {
		return ""
	}
	fg, bg := st.fg, st.bg
	if fg == "" {
		fg = "#e8e6df"
	}
	if bg == "" {
		bg = "transparent"
	}
	part := func(dir string, eighths int) string {
		p := fmt.Sprintf("%.1f%%", float64(eighths)*12.5)
		return fmt.Sprintf("linear-gradient(to %s, %s %s, %s %s)", dir, fg, p, bg, p)
	}
	switch r := rs[0]; {
	case r == 0x2588:
		return fg
	case r == 0x2580:
		return part("bottom", 4)
	case r >= 0x2581 && r <= 0x2587: // lower one to seven eighths
		return part("top", int(r-0x2580))
	case r >= 0x2589 && r <= 0x258F: // left seven to one eighths
		return part("right", int(0x2590-r))
	case r == 0x2590:
		return part("left", 4)
	case r >= 0x2591 && r <= 0x2593: // light, medium and dark shade
		return fmt.Sprintf("color-mix(in srgb, %s %d%%, %s)", fg, int(r-0x2590)*25, bg)
	}
	return ""
}

func (st cellStyle) apply(params string) cellStyle {
	ps := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	if len(ps) == 0 {
		return cellStyle{}
	}
	for i := 0; i < len(ps); i++ {
		n, _ := strconv.Atoi(ps[i])
		switch {
		case n == 0:
			st = cellStyle{}
		case n == 1:
			st.bold = true
		case n == 2:
			st.faint = true
		case n == 3:
			st.italic = true
		case n == 4:
			st.under = true
		case n == 7:
			st.rev = true
		case n == 9:
			st.strike = true
		case n == 22:
			st.bold, st.faint = false, false
		case n == 23:
			st.italic = false
		case n == 24:
			st.under = false
		case n == 27:
			st.rev = false
		case n == 29:
			st.strike = false
		case n == 39:
			st.fg = ""
		case n == 49:
			st.bg = ""
		case (n == 38 || n == 48) && i+4 < len(ps) && ps[i+1] == "2":
			r, _ := strconv.Atoi(ps[i+2])
			g, _ := strconv.Atoi(ps[i+3])
			bl, _ := strconv.Atoi(ps[i+4])
			c := fmt.Sprintf("#%02x%02x%02x", r, g, bl)
			if n == 38 {
				st.fg = c
			} else {
				st.bg = c
			}
			i += 4
		case (n == 38 || n == 48) && i+2 < len(ps) && ps[i+1] == "5":
			i += 2 // 256-colour codes do not occur with a true-colour profile
		}
	}
	return st
}

func (st cellStyle) css() string {
	fg, bg := st.fg, st.bg
	if st.rev {
		fg, bg = bg, fg
		if fg == "" {
			fg = "#0b0e1a"
		}
		if bg == "" {
			bg = "#e8e6df"
		}
	}
	var css []string
	if fg != "" {
		css = append(css, "color:"+fg)
	}
	if bg != "" {
		css = append(css, "background:"+bg)
	}
	if st.bold {
		css = append(css, "font-weight:700")
	}
	if st.faint {
		css = append(css, "opacity:.6")
	}
	if st.italic {
		css = append(css, "font-style:italic")
	}
	var deco []string
	if st.under {
		deco = append(deco, "underline")
	}
	if st.strike {
		deco = append(deco, "line-through")
	}
	if len(deco) > 0 {
		css = append(css, "text-decoration:"+strings.Join(deco, " "))
	}
	if len(css) == 0 {
		return ""
	}
	return ` style="` + strings.Join(css, ";") + `"`
}
