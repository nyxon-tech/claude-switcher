package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

const sample = `{"sessionId":"local_aaa","cliSessionId":"s2","priorCliSessionIds":["s0","s1"],"cwd":"C:\\work\\proj","title":"بازبینی <div> & co","lastActivityAt":1789898700000,"createdAt":1789898400000,"isArchived":true,"model":"claude-opus-5","rewindEdges":[{"id":1,"at":2.5e3}],"isStarred":true,"future":{"x":null}}`

func TestCardRoundTripIsByteForByte(t *testing.T) {
	c, err := ParseCard([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(c.Marshal()); got != sample {
		t.Fatalf("round trip changed the card:\n got %s\nwant %s", got, sample)
	}
}

func TestCardCompactsPrettyInput(t *testing.T) {
	c, err := ParseCard([]byte("{\n  \"a\": [1, 2],\n  \"b\": {\"c\": \"x y\"}\n}"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(c.Marshal()); got != `{"a":[1,2],"b":{"c":"x y"}}` {
		t.Fatalf("got %s", got)
	}
}

func TestCardFields(t *testing.T) {
	c, err := ParseCard([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if c.ID() != "local_aaa" || c.Session() != "s2" || c.Cwd() != `C:\work\proj` || c.Model() != "claude-opus-5" {
		t.Errorf("strings: %q %q %q %q", c.ID(), c.Session(), c.Cwd(), c.Model())
	}
	if p := c.PriorSessions(); len(p) != 2 || p[1] != "s1" {
		t.Errorf("prior = %v", p)
	}
	if c.LastActivity() != 1789898700000 || c.Created() != 1789898400000 || !c.Archived() {
		t.Errorf("numbers: %d %d %v", c.LastActivity(), c.Created(), c.Archived())
	}
	empty, _ := ParseCard([]byte(`{"title":5}`))
	if empty.Title() != "" || empty.LastActivity() != 0 || empty.Archived() || empty.PriorSessions() != nil {
		t.Error("missing or mistyped fields should read as zero")
	}
}

func TestCardSetKeepsOrder(t *testing.T) {
	c, _ := ParseCard([]byte(`{"a":1,"title":"old","z":true}`))
	if err := c.Set("title", "<new>"); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("added", 2); err != nil {
		t.Fatal(err)
	}
	if got := string(c.Marshal()); got != `{"a":1,"title":"<new>","z":true,"added":2}` {
		t.Fatalf("got %s", got)
	}
}

func TestCardIDComesFromFileName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local_file.json")
	os.WriteFile(path, []byte(`{"sessionId":"local_other"}`), 0o600)
	c, err := ReadCard(path)
	if err != nil || c.ID() != "local_file" {
		t.Fatalf("id = %q, %v", c.ID(), err)
	}
}

func TestParseCardRejectsNonObjects(t *testing.T) {
	for _, in := range []string{"", "[]", `{"a":`, "null"} {
		if _, err := ParseCard([]byte(in)); err == nil {
			t.Errorf("%q parsed", in)
		}
	}
}

func TestNewCardHasV2Fields(t *testing.T) {
	start := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	m := transcript.Meta{Session: "s1", Cwd: `C:\work\proj`, Title: "  Fix\nthe bug ", Model: "claude-opus-5", Start: start, End: start.Add(5 * time.Minute)}
	c := NewCard(m)
	want := `{"sessionId":"local_UUID","cliSessionId":"s1","cwd":"C:\\work\\proj","originCwd":"C:\\work\\proj","lastFocusedAt":1789898700000,"createdAt":1789898400000,"lastActivityAt":1789898700000,"isArchived":false,"title":"Fix the bug","titleSource":"auto","permissionMode":"default","remoteMcpServersConfig":[],"alwaysAllowedReasons":[],"sessionPermissionUpdates":[],"spawnSeed":{},"model":"claude-opus-5"}`
	uuid := regexp.MustCompile(`local_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
	if got := uuid.ReplaceAllString(string(c.Marshal()), "local_UUID"); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if !strings.HasPrefix(c.ID(), "local_") || c.ID() == NewCard(m).ID() {
		t.Error("each new card needs its own id")
	}
	if !json.Valid(c.Marshal()) {
		t.Error("invalid json")
	}
}

func TestNewCardModelAndTitleFallbacks(t *testing.T) {
	long := strings.Repeat("ب", 100)
	tests := []struct {
		meta      transcript.Meta
		title     string
		withModel bool
	}{
		{transcript.Meta{Title: "t", Model: "deepseek-v4-pro"}, "t", false},
		{transcript.Meta{FirstPrompt: "build the login page", Model: "claude-fable-5"}, "build the login page", true},
		{transcript.Meta{}, RecoveredTitle, false},
		{transcript.Meta{Title: long}, strings.Repeat("ب", 79) + "…", false},
	}
	for _, tt := range tests {
		c := NewCard(tt.meta)
		if c.Title() != tt.title {
			t.Errorf("title = %q, want %q", c.Title(), tt.title)
		}
		if (c.Model() != "") != tt.withModel {
			t.Errorf("model = %q for %q", c.Model(), tt.meta.Model)
		}
		if c.Created() != 0 {
			t.Errorf("zero start should give createdAt 0, got %d", c.Created())
		}
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local_x.json")
	for _, text := range []string{"first", "second"} {
		if err := WriteFileAtomic(path, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	entries, _ := os.ReadDir(dir)
	if string(data) != "second" || len(entries) != 1 {
		t.Fatalf("content %q, %d files", data, len(entries))
	}
}

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSpaces(t *testing.T) {
	data := t.TempDir()
	root := filepath.Join(data, "claude-code-sessions")
	dir := filepath.Join(root, "acct1", "org1")
	put(t, filepath.Join(dir, "local_a.json"), `{"cliSessionId":"s1"}`)
	put(t, filepath.Join(dir, "local_b.json.tmp"), `{"cliSessionId":"half`)
	put(t, filepath.Join(dir, "local_c.json"), `not json`)
	put(t, filepath.Join(dir, "deleted_s9"), `1789000000000`)
	put(t, filepath.Join(dir, "scheduled-tasks.json"), `{}`)
	put(t, filepath.Join(root, "acct2", "org2", "backlog", "tasks.json"), `{}`)
	put(t, filepath.Join(root, "stray-file"), "")

	spaces, err := Spaces(data)
	if err != nil || len(spaces) != 2 {
		t.Fatalf("spaces = %v, %v", spaces, err)
	}
	if s := spaces[0]; s.Account != "acct1" || s.Org != "org1" || s.Dir != dir || s.Linked() {
		t.Errorf("space = %+v", s)
	}
	cards, _ := spaces[0].Cards()
	if len(cards) != 1 || cards[0].Session() != "s1" {
		t.Errorf("cards = %d", len(cards))
	}
	if tomb, _ := spaces[0].Tombstones(); len(tomb) != 1 || tomb[0] != "s9" {
		t.Errorf("tombstones = %v", tomb)
	}
	if none, err := Spaces(t.TempDir()); none != nil || err != nil {
		t.Errorf("no sessions folder: %v %v", none, err)
	}
}

func TestLinkedSpace(t *testing.T) {
	data := t.TempDir()
	real := filepath.Join(data, "claude-code-sessions", "acct1", "org1")
	put(t, filepath.Join(real, "local_a.json"), `{}`)
	link := filepath.Join(data, "claude-code-sessions", "acct2", "org1")
	os.MkdirAll(filepath.Dir(link), 0o700)
	makeLink(t, real, link)

	spaces, err := Spaces(data)
	if err != nil || len(spaces) != 2 {
		t.Fatalf("spaces = %v, %v", spaces, err)
	}
	if spaces[0].Linked() || !spaces[1].Linked() {
		t.Error("only the linked list should report Linked")
	}
	if cards, _ := spaces[1].Cards(); len(cards) != 1 {
		t.Error("cards behind a link should still be read")
	}
}

func TestLiveAccount(t *testing.T) {
	data := t.TempDir()
	if LiveAccount(data) != "" {
		t.Error("no config.json should mean signed out")
	}
	put(t, filepath.Join(data, "config.json"), `{"oauth":{"token":"x"},"lastKnownAccountUuid":"acct1"}`)
	if got := LiveAccount(data); got != "acct1" {
		t.Errorf("got %q", got)
	}
}

func TestMarshalHasNoBOM(t *testing.T) {
	if b := NewCard(transcript.Meta{Title: "x"}).Marshal(); bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("byte order mark")
	}
}
