package transcript

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	t1 = "2026-09-01T10:00:00.000Z"
	t2 = "2026-09-01T10:05:00.000Z"
	t3 = "2026-09-02T08:30:00.000Z"
)

func TestTitlePrecedence(t *testing.T) {
	long := strings.Repeat("ab ", 50)
	tests := []struct {
		name  string
		lines []any
		want  string
	}{
		{"last custom title", []any{meta("custom-title", "customTitle", "Old"), meta("agent-name", "agentName", "Agent"),
			meta("ai-title", "aiTitle", "AI"), meta("custom-title", "customTitle", "New")}, "New"},
		{"agent name", []any{meta("ai-title", "aiTitle", "AI"), meta("agent-name", "agentName", "Agent")}, "Agent"},
		{"ai title", []any{meta("ai-title", "aiTitle", "AI 1"), meta("ai-title", "aiTitle", "AI 2")}, "AI 2"},
		{"first prompt, cut", []any{user(t1, long), user(t2, "second")}, long[:79] + "…"},
		{"none", []any{meta("mode", "mode", "normal")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, tt.lines...)
			for _, read := range []func(string) (Meta, error){ReadMeta, QuickMeta} {
				m, err := read(path)
				if err != nil {
					t.Fatal(err)
				}
				if m.Title != tt.want {
					t.Errorf("title = %q, want %q", m.Title, tt.want)
				}
			}
		})
	}
}

func TestPromptRules(t *testing.T) {
	image := obj{"type": "image", "source": obj{"type": "base64", "media_type": "image/png", "data": "AAAA"}}
	tests := []struct {
		name string
		line obj
		want string // "" when the line is not a prompt
	}{
		{"plain text on one line", user(t1, "  fix the\n\n bug  "), "fix the bug"},
		{"text block", user(t1, []obj{text("hello")}), "hello"},
		{"image first", user(t1, []obj{image, text("what is this")}), "[image] what is this"},
		{"meta", msg("user", t1, obj{"isMeta": true, "message": obj{"content": "x"}}), ""},
		{"compact summary", msg("user", t1, obj{"isCompactSummary": true, "message": obj{"content": "x"}}), ""},
		{"transcript only", msg("user", t1, obj{"isVisibleInTranscriptOnly": true, "message": obj{"content": "x"}}), ""},
		{"tool result", user(t1, []obj{{"type": "tool_result", "tool_use_id": "toolu_1", "content": "out"}}), ""},
		{"tool result and text", user(t1, []obj{{"type": "tool_result", "tool_use_id": "toolu_1", "content": "out"}, text("and this")}), "and this"},
		{"empty command", user(t1, "<command-name></command-name><command-args></command-args>"), ""},
		{"caveat", user(t1, "<local-command-caveat>Caveat</local-command-caveat>"), ""},
		{"command output", user(t1, "<local-command-stdout>ok</local-command-stdout>"), ""},
		{"task notification", user(t1, "<task-notification>done</task-notification>"), ""},
		{"interrupted", user(t1, "[Request interrupted by user]"), ""},
		{"command with args", user(t1, "<command-message>review</command-message>\n<command-name>/review</command-name>\n<command-args>the  diff</command-args>"), "the diff"},
		{"command without args", user(t1, "<command-name>/clear</command-name>\n<command-message>clear</command-message>\n<command-args></command-args>"), "/clear"},
		{"command message only", user(t1, "<command-message>dicex</command-message>"), "/dicex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ReadMeta(fixture(t, tt.line))
			if err != nil {
				t.Fatal(err)
			}
			wantPrompts := 0
			if tt.want != "" {
				wantPrompts = 1
			}
			if m.FirstPrompt != tt.want || m.Prompts != wantPrompts {
				t.Errorf("first prompt %q (%d prompts), want %q (%d)", m.FirstPrompt, m.Prompts, tt.want, wantPrompts)
			}
		})
	}
}

func TestReadMetaCounts(t *testing.T) {
	path := fixture(t,
		meta("custom-title", "customTitle", "Chat"),
		user(t2, "first"),
		reply(t2, "msg_1", "claude-opus-5", text("a")),
		reply(t2, "msg_1", "claude-opus-5", tool("Bash")),
		user(t3, []obj{{"type": "tool_result", "tool_use_id": "toolu_1", "content": "out"}}),
		reply(t1, "msg_2", "claude-sonnet-5", text("b")), // copied history: older than the lines above
		queued(t3, "prompt", "queued one"),
		queued(t3, "task-notification", "<task-notification>x</task-notification>"),
		reply(t3, "msg_x", synthetic, text("API Error")),
		meta("last-prompt", "lastPrompt", "queued one"),
	)
	m, err := ReadMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse(time.RFC3339, t1)
	end, _ := time.Parse(time.RFC3339, t3)
	if m.Prompts != 2 || m.Replies != 2 || m.Model != "claude-sonnet-5" || !m.Start.Equal(start) || !m.End.Equal(end) {
		t.Errorf("prompts %d replies %d model %q span %v..%v", m.Prompts, m.Replies, m.Model, m.Start, m.End)
	}
	if m.Session != sid || m.Cwd != "/work/app" || m.Branch != "main" || m.FirstPrompt != "first" || m.LastPrompt != "queued one" || m.Title != "Chat" {
		t.Errorf("meta %+v", m)
	}
	if !m.HasMessages() {
		t.Error("HasMessages = false")
	}
}

func TestHugeLine(t *testing.T) {
	big := strings.Repeat("x", 5<<20+123)
	path := fixture(t,
		user(t1, "before"),
		msg("user", t2, obj{"message": obj{"content": []obj{{"type": "tool_result", "content": big}}}, "toolUseResult": obj{"stdout": big}}),
		reply(t2, "msg_1", "claude-opus-5", text("after")),
		meta("custom-title", "customTitle", "Big"),
	)
	m, err := ReadMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Prompts != 1 || m.Replies != 1 || m.Title != "Big" {
		t.Errorf("ReadMeta: %d prompts, %d replies, title %q", m.Prompts, m.Replies, m.Title)
	}
	q, err := QuickMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if q.Title != "Big" || q.FirstPrompt != "before" || q.Model != "claude-opus-5" {
		t.Errorf("QuickMeta: %+v", q)
	}
	msgs, err := ReadMessages(path, 0)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("ReadMessages: %d messages, %v", len(msgs), err)
	}
}

func TestQuickMetaHeadTail(t *testing.T) {
	filler := strings.Repeat("y", 20<<10)
	lines := []any{meta("custom-title", "customTitle", "Old"), user(t1, "first prompt")}
	for i := range 40 { // 800 KB of replies between head and tail
		lines = append(lines, reply(t2, "msg_"+strconv.Itoa(i), "claude-opus-4-8", text(filler)))
	}
	lines = append(lines, reply(t3, "msg_last", "claude-opus-5", text("done")),
		meta("custom-title", "customTitle", "New"), meta("last-prompt", "lastPrompt", "last one"))
	m, err := QuickMeta(fixture(t, lines...))
	if err != nil {
		t.Fatal(err)
	}
	end, _ := time.Parse(time.RFC3339, t3)
	if m.Title != "New" || m.FirstPrompt != "first prompt" || m.LastPrompt != "last one" || m.Cwd != "/work/app" ||
		m.Model != "claude-opus-5" || !m.End.Equal(end) || m.Prompts != 0 || m.Replies != 0 {
		t.Errorf("QuickMeta: %+v", m)
	}
}

// With no message line in the head or the tail, QuickMeta reads the whole file.
func TestQuickMetaFallback(t *testing.T) {
	huge := obj{"type": "file-history-snapshot", "snapshot": strings.Repeat("z", headSize+1)}
	lines := []any{huge, user(t1, "hidden in the middle")}
	for range 10 {
		lines = append(lines, obj{"type": "mode", "mode": strings.Repeat("m", 10<<10)})
	}
	m, err := QuickMeta(fixture(t, lines...))
	if err != nil {
		t.Fatal(err)
	}
	if m.Cwd != "/work/app" || m.Title != "hidden in the middle" {
		t.Errorf("fallback: %+v", m)
	}
}

func TestIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	other := "6a1e2b3c-4d5e-4f60-8a7b-9c0d1e2f3a4b"
	keep := writeJSONL(t, filepath.Join(root, "C--a", sid+".jsonl"))
	keep2 := writeJSONL(t, filepath.Join(root, "C--b", other+".jsonl"))
	for _, skip := range []string{
		filepath.Join("C--a", sid, "subagents", "agent-a1.jsonl"),
		filepath.Join("C--a", ".orphaned-"+other+".jsonl"),
		filepath.Join("C--a", other+".orphaned-1.jsonl"),
		filepath.Join("C--a", other+".jsonl.superseded-1"),
		filepath.Join("C--a", "notes.jsonl"),
		filepath.Join("C--a", "memory", other+".jsonl"),
	} {
		writeJSONL(t, filepath.Join(root, skip))
	}
	got, err := Index(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[sid] != keep || got[other] != keep2 {
		t.Errorf("Index = %v", got)
	}
	if got, err := Index(filepath.Join(root, "missing")); err != nil || len(got) != 0 {
		t.Errorf("missing folder: %v, %v", got, err)
	}
}

// A line that starts exactly where the tail begins must not be dropped with the torn one.
func TestQuickMetaTailBoundary(t *testing.T) {
	lines := []any{user(t1, "first")}
	for i := range 20 {
		lines = append(lines, reply(t2, "msg_"+strconv.Itoa(i), "claude-opus-5", text(strings.Repeat("y", 20<<10))))
	}
	last := `{"type":"custom-title","customTitle":"Edge","sessionId":"` + sid + `","pad":"`
	last += strings.Repeat("p", tailSize-len(last)-len(`"}`)-1) + `"}`
	m, err := QuickMeta(fixture(t, append(lines, last)...))
	if err != nil || m.Title != "Edge" {
		t.Errorf("title %q, %v", m.Title, err)
	}
}

func TestDamagedFiles(t *testing.T) {
	torn := fixture(t, user(t1, "hi"), reply(t2, "msg_1", "claude-opus-5", text("a")), `{"type":"assistant","message":{"id":"msg_2"`)
	if m, err := ReadMeta(torn); err != nil || m.Prompts != 1 || m.Replies != 1 {
		t.Errorf("torn last line: %+v, %v", m, err)
	}
	empty := fixture(t)
	if m, err := QuickMeta(empty); err != nil || m.HasMessages() || m.Title != "" || m.Session != sid {
		t.Errorf("empty file: %+v, %v", m, err)
	}
	dir := t.TempDir()
	for _, missing := range []string{filepath.Join(dir, sid+".jsonl"), filepath.Join(dir, "gone", sid+".jsonl")} {
		if _, err := QuickMeta(missing); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("QuickMeta(missing) = %v", err)
		}
		if _, err := ReadMessages(missing, 0); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadMessages(missing) = %v", err)
		}
	}
}

// Claude must be able to rename or delete a transcript while it is being read (Windows locks
// files opened without delete sharing).
func TestReadKeepsTranscriptMovable(t *testing.T) {
	path := fixture(t, user(t1, "hi"))
	f, err := openShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Rename(path, path+".superseded-1"); err != nil {
		t.Errorf("rename while reading: %v", err)
	}
	if err := os.Remove(path + ".superseded-1"); err != nil {
		t.Errorf("delete while reading: %v", err)
	}
}
