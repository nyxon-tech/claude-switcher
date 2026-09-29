package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollect(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	other, stub := "6a1e2b3c-4d5e-4f60-8a7b-9c0d1e2f3a4b", "7b2f3c4d-5e6f-4a71-9b8c-0d1e2f3a4b5c"
	elsewhere := func(l obj) obj { l["cwd"] = "/work/other"; return l }
	writeJSONL(t, filepath.Join(root, "C--a", sid+".jsonl"),
		user(t1, "hi"),
		reply(t1, "msg_1", "claude-opus-5", text("a")),
		reply(t1, "msg_1", "claude-opus-5", tool("Bash")), // same message, second block
		reply(t2, "msg_2", "claude-opus-5", text("b")),
		reply(t2, "msg_s", synthetic, text("API Error")),
	)
	writeJSONL(t, filepath.Join(root, "C--a", sid, "subagents", "agent-a1.jsonl"),
		reply(t2, "msg_3", "claude-haiku-4-5", text("c")),
		reply(t1, "msg_1", "claude-opus-5", text("a")), // copied from the main file
	)
	mainC := writeJSONL(t, filepath.Join(root, "C--b", other+".jsonl"),
		elsewhere(reply(t2, "msg_2", "claude-opus-5", text("b"))), // copied history
		elsewhere(reply(t3, "msg_4", "claude-sonnet-5", text("d"))),
	)
	writeJSONL(t, filepath.Join(root, "C--b", stub+".jsonl"), meta("custom-title", "customTitle", "Empty"))

	cache := filepath.Join(t.TempDir(), "cache", "usage.gob")
	calls := 0
	cold, err := Collect(root, cache, func(done, total int) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	one := Usage{Input: 1, Output: 10, CacheWrite: 100, CacheRead: 1000, Messages: 1}
	if cold.ByModel["claude-opus-5"] != one.Add(one) || cold.ByModel["claude-haiku-4-5"] != one ||
		cold.ByModel["claude-sonnet-5"] != one || len(cold.ByModel) != 3 {
		t.Errorf("by model: %+v", cold.ByModel)
	}
	if cold.ByProject["/work/app"].Messages != 3 || cold.ByProject["/work/other"].Messages != 1 {
		t.Errorf("by project: %+v", cold.ByProject)
	}
	days := 0
	for _, u := range cold.ByDay {
		days += u.Messages
	}
	if days != 4 || cold.Sessions != 2 || cold.FilesRead != 4 || cold.FilesTotal != 4 || calls != 4 {
		t.Errorf("days %d sessions %d read %d/%d progress calls %d", days, cold.Sessions, cold.FilesRead, cold.FilesTotal, calls)
	}
	if cold.First.Format(time.RFC3339) != "2026-09-01T10:00:00Z" || cold.Last.Format(time.RFC3339) != "2026-09-02T08:30:00Z" {
		t.Errorf("span %v..%v", cold.First, cold.Last)
	}

	warm, err := Collect(root, cache, nil)
	if err != nil {
		t.Fatal(err)
	}
	if warm.FilesRead != 0 {
		t.Errorf("warm run parsed %d files", warm.FilesRead)
	}
	warm.FilesRead = cold.FilesRead
	if a, b := asJSON(t, cold), asJSON(t, warm); a != b {
		t.Errorf("warm stats differ:\n%s\n%s", a, b)
	}

	f, err := os.OpenFile(mainC, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(asJSON(t, reply(t3, "msg_5", "claude-sonnet-5", text("e"))) + "\n")
	f.Close()
	changed, err := Collect(root, cache, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed.FilesRead != 1 || changed.ByModel["claude-sonnet-5"].Messages != 2 {
		t.Errorf("after append: read %d, sonnet %+v", changed.FilesRead, changed.ByModel["claude-sonnet-5"])
	}
}

func TestCollectMissingFolder(t *testing.T) {
	st, err := Collect(filepath.Join(t.TempDir(), "none"), "", nil)
	if err != nil || st.FilesTotal != 0 || st.ByModel == nil {
		t.Errorf("%+v, %v", st, err)
	}
}

// ~/.claude/projects is often a symlink (dotfiles) or a junction.
func TestCollectLinkedFolder(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "real", "C--a", sid+".jsonl"), reply(t1, "msg_1", "claude-opus-5", text("a")))
	link := filepath.Join(dir, "projects")
	if err := os.Symlink(filepath.Join(dir, "real"), link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	st, err := Collect(link, "", nil)
	if err != nil || st.FilesTotal != 1 || st.Sessions != 1 {
		t.Errorf("%+v, %v", st, err)
	}
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
