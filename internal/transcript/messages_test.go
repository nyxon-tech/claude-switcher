package transcript

import (
	"slices"
	"strings"
	"testing"
)

func TestReadMessages(t *testing.T) {
	path := fixture(t,
		meta("custom-title", "customTitle", "Chat"),
		user(t1, "<command-name>/review</command-name>\n<command-args>the diff</command-args>"),
		reply(t1, "msg_1", "claude-opus-5", obj{"type": "thinking", "thinking": "hmm"}),
		reply(t1, "msg_1", "claude-opus-5", text("Looking.")),
		reply(t1, "msg_1", "claude-opus-5", tool("Bash")),
		user(t2, []obj{{"type": "tool_result", "tool_use_id": "toolu_1", "content": "out"}}),
		`{"type":"user","uuid":"raw","message":{"content":[{"tool_use_id":"toolu_2","type":"tool_result","content":"out"}]}}`,
		reply(t1, "msg_1", "claude-opus-5", text("Found it."), tool("Edit")),
		msg("system", t2, obj{"subtype": "api_error"}),
		reply(t2, "msg_2", synthetic, text("API Error")),
		queued(t2, "prompt", "and the tests"),
		reply(t3, "msg_3", "claude-sonnet-5", text("Done.")),
	)
	msgs, err := ReadMessages(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Message{
		{Role: "user", Text: "/review the diff"},
		{Role: "assistant", Text: "Looking.\n\nFound it.", Tools: []string{"Bash", "Edit"}, Model: "claude-opus-5"},
		{Role: "user", Text: "and the tests"},
		{Role: "assistant", Text: "Done.", Model: "claude-sonnet-5"},
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d messages, want %d: %+v", len(msgs), len(want), msgs)
	}
	for i, w := range want {
		g := msgs[i]
		if g.Role != w.Role || g.Text != w.Text || !slices.Equal(g.Tools, w.Tools) || g.Model != w.Model || g.Time.IsZero() {
			t.Errorf("message %d = %+v, want %+v", i, g, w)
		}
	}
	last, err := ReadMessages(path, 1)
	if err != nil || len(last) != 1 || last[0].Text != "Done." {
		t.Errorf("limit 1: %+v, %v", last, err)
	}
}

func TestExportEscaping(t *testing.T) {
	m := Meta{Title: "رفع باگ <b>", Cwd: `C:\work\app`, Model: "claude-opus-5"}
	msgs := []Message{
		{Role: "user", Text: "run <script>alert(1)</script> and `<b>` please\n```html\n<div>\n```"},
		{Role: "assistant", Text: "```go\nx := 1 < 2", Tools: []string{"Bash"}, Model: "claude-opus-5"},
		{Role: "user", Text: "سلام"},
	}
	var md strings.Builder
	if err := WriteMarkdown(&md, m, msgs); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# رفع باگ &lt;b>", "run &lt;script>alert(1)&lt;/script> and `<b>` please", "```html\n<div>\n```",
		"x := 1 < 2\n```", "*Tools: `Bash`*", "- Models: claude-opus-5"} {
		if !strings.Contains(md.String(), want) {
			t.Errorf("markdown lacks %q:\n%s", want, md.String())
		}
	}
	if strings.Contains(md.String(), "<script>") {
		t.Error("markdown keeps a raw <script>")
	}

	var page strings.Builder
	if err := WriteHTML(&page, m, msgs); err != nil {
		t.Fatal(err)
	}
	html := page.String()
	if strings.Contains(html, "<script") || strings.Contains(html, "<div>") {
		t.Error("html keeps raw markup from the chat")
	}
	for _, want := range []string{`<html lang="fa">`, "رفع باگ &lt;b&gt;", "&lt;script&gt;alert(1)&lt;/script&gt;", `<article class="user" dir="auto">`,
		`<article class="assistant" dir="auto">`, "<li>Bash</li>", "white-space: pre-wrap",
		`href="https://github.com/nyxon-tech/claude-switcher">Exported with Claude Switcher by Nyxon</a>`} {
		if !strings.Contains(html, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	if strings.Count(html, `dir="auto"`) < len(msgs) {
		t.Error("every message needs dir=auto")
	}
}

// Markdown viewers must never see raw HTML outside code: every case where this reading of
// code could differ from a viewer's escapes instead.
func TestMarkdownEscape(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"text", "a <b> c", "a &lt;b> c"},
		{"code span", "use `Vec<T>` here", "use `Vec<T>` here"},
		{"escaped backtick", "\\`<i>x</i>`", "\\`&lt;i>x&lt;/i>`"},
		{"span over two lines", "a `b\nc` <i>x</i> `d`", "a `b\nc` &lt;i>x&lt;/i> `d`"},
		{"blank line ends the paragraph", "a `b\n\n`<b>`", "a `b\n\n`<b>`"},
		{"fence", "```html\n<div>\n```\n<p>", "```html\n<div>\n```\n&lt;p>"},
		{"longer fence", "````\n```\n<b>\n````", "````\n```\n<b>\n````"},
		{"fence left open", "~~~\n<b>", "~~~\n<b>\n~~~"},
		{"four spaces is no fence", "text\n    ```\n<i>", "text\n    ```\n&lt;i>"},
		{"backtick in info string", "``` a`b\n<i>", "``` a`b\n&lt;i>"},
		{"closer with info string", "```\n```go\n<b>\n```", "```\n```go\n<b>\n```"},
		{"fence in list item", "1. run\n   ```sh\n   a < b\n   ```\n<i>", "1. run\n   ```sh\n   a < b\n   ```\n&lt;i>"},
		{"list item ends the fence", "- a\n  ```\n<i>\n  ```\n<b>", "- a\n  ```\n&lt;i>\n  ```\n&lt;b>"},
		{"indented fence left open", "- a\n  ```\n  <b>", "- a\n  ```\n  <b>\n  ```"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mdEscape(tt.in); got != tt.want {
				t.Errorf("mdEscape(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
	var md strings.Builder
	if err := WriteMarkdown(&md, Meta{Title: "```x <b>"}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(md.String(), "# ```x &lt;b>\n") || strings.Count(md.String(), "```") != 1 {
		t.Errorf("title: %q", md.String())
	}
}
