package store

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// hash fingerprints every file under dir (names and contents).
func hash(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(path)
			rel, _ := filepath.Rel(dir, path)
			lines = append(lines, fmt.Sprintf("%s=%x", rel, sha256.Sum256(data)))
		}
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, ";")
}

func TestValidName(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"work", true},
		{"my work 2", true},
		{"a", true},
		{"کار", true},
		{"کار‌شخصی", true},
		{"team.alpha_1-b", true},
		{strings.Repeat("x", 40), true},
		{strings.Repeat("x", 41), false},
		{"", false},
		{"_hidden", false},
		{"trailing.", false},
		{"trailing ", false},
		{" leading", false},
		{"a/b", false},
		{`a\b`, false},
		{"a:b", false},
		{"‌لبه", false},
		{"CON", false},
		{"nul", false},
		{"Nul.work", false},
		{"com1", false},
		{"LPT9", false},
		{"COM10", true},
		{"console", true},
	}
	for _, tt := range tests {
		if got := ValidName(tt.name); got != tt.ok {
			t.Errorf("ValidName(%q) = %v, want %v", tt.name, got, tt.ok)
		}
	}
}

func TestProfiles(t *testing.T) {
	v := Vault{Dir: t.TempDir()}
	if ps, err := v.Profiles(); ps != nil || err != nil {
		t.Fatalf("empty vault: %v %v", ps, err)
	}
	put(t, filepath.Join(v.Dir, "work", "config.json"), "{}")
	put(t, filepath.Join(v.Dir, "work", "_account"), "acct-work\r\n")
	put(t, filepath.Join(v.Dir, "_journal", "x", "config.json"), "{}")
	put(t, filepath.Join(v.Dir, "half", "Preferences"), "")
	put(t, filepath.Join(v.Dir, "_current_profile"), "work")

	ps, err := v.Profiles()
	if err != nil || len(ps) != 1 || ps[0].Name != "work" || ps[0].Account != "acct-work" || ps[0].Saved.IsZero() {
		t.Fatalf("profiles = %+v, %v", ps, err)
	}
	if p, ok := v.Find("WORK"); !ok || p.Name != "work" {
		t.Error("Find should ignore case")
	}
	if _, ok := v.Find("half"); ok {
		t.Error("a folder without config.json is not a profile")
	}
	if v.Current() != "work" {
		t.Errorf("current = %q", v.Current())
	}

	persian := "کار"
	if err := v.Rename("work", persian); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.Find(persian); !ok || v.Current() != persian {
		t.Error("rename should move the profile and the current marker")
	}
	if err := v.Remove(persian); err != nil {
		t.Fatal(err)
	}
	if ps, _ := v.Profiles(); len(ps) != 0 {
		t.Error("remove left the profile")
	}
}

var items = []string{"config.json", "Preferences", "DIPS", "DIPS-wal", "Cookies", "Local Storage", "Network", "IndexedDB"}

func fillLogin(t *testing.T, data, who string) {
	put(t, filepath.Join(data, "config.json"), `{"lastKnownAccountUuid":"`+who+`"}`)
	for _, f := range []string{"Preferences", "DIPS", "DIPS-wal", "Cookies"} {
		put(t, filepath.Join(data, f), who+"-"+f)
	}
	for _, f := range []string{"Local Storage/leveldb/000003.log", "Network/Cookies", "IndexedDB/https_claude.ai_0.indexeddb.leveldb/LOG"} {
		put(t, filepath.Join(data, filepath.FromSlash(f)), who+"-"+f)
	}
}

// login fingerprints only the login items of a data folder.
func login(t *testing.T, data string) string {
	var parts []string
	for _, it := range items {
		p := filepath.Join(data, it)
		st, err := os.Stat(p)
		switch {
		case err != nil:
			parts = append(parts, it+"=-")
		case st.IsDir():
			parts = append(parts, it+"/"+hash(t, p))
		default:
			parts = append(parts, it+"="+read(t, p))
		}
	}
	return strings.Join(parts, ";")
}

func TestLoginSaveClearRestore(t *testing.T) {
	root := t.TempDir()
	data, v := filepath.Join(root, "Claude"), Vault{Dir: filepath.Join(root, "vault")}
	fillLogin(t, data, "work")
	put(t, filepath.Join(data, "Local State"), "shared-key")
	put(t, filepath.Join(data, "claude_desktop_config.json"), "{}")
	shared := hash(t, data)
	workLogin := login(t, data)

	if err := v.SaveLogin("work", data, items); err != nil {
		t.Fatal(err)
	}
	if got := login(t, filepath.Join(v.Dir, "work")); got != workLogin {
		t.Fatalf("saved login differs:\n%s\n%s", got, workLogin)
	}
	if err := ClearLogin(data, items); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if _, err := os.Stat(filepath.Join(data, it)); err == nil {
			t.Errorf("%s survived ClearLogin", it)
		}
	}

	// another account signs in, without a DIPS-wal and with an extra Local Storage file
	fillLogin(t, data, "personal")
	os.Remove(filepath.Join(data, "DIPS-wal"))
	put(t, filepath.Join(data, "Local Storage", "extra"), "x")
	personalLogin := login(t, data)
	if err := v.SaveLogin("personal", data, items); err != nil {
		t.Fatal(err)
	}

	if err := v.RestoreLogin("work", data, items); err != nil {
		t.Fatal(err)
	}
	if got := login(t, data); got != workLogin {
		t.Fatalf("restore is not byte for byte:\n%s\n%s", got, workLogin)
	}
	if err := v.RestoreLogin("personal", data, items); err != nil {
		t.Fatal(err)
	}
	if got := login(t, data); got != personalLogin {
		t.Fatal("restoring personal differs")
	}
	if _, err := os.Stat(filepath.Join(data, "DIPS-wal")); err == nil {
		t.Error("a stale DIPS-wal from the other account survived")
	}
	if !strings.Contains(hash(t, data), "Local State") || read(t, filepath.Join(data, "Local State")) != "shared-key" || shared == "" {
		t.Error("files outside the login items must not change")
	}

	// saving again over an existing profile drops what Desktop no longer has
	os.RemoveAll(filepath.Join(data, "IndexedDB"))
	if err := v.SaveLogin("personal", data, items); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(v.Dir, "personal", "IndexedDB")); err == nil {
		t.Error("a folder missing at the source should be removed from the profile")
	}
}

func TestLoginRefusesEmptyPaths(t *testing.T) {
	v := Vault{Dir: t.TempDir()}
	if v.SaveLogin("x", "", items) == nil || v.RestoreLogin("x", "", items) == nil || ClearLogin("", items) == nil {
		t.Fatal("an empty data folder would resolve to the working directory")
	}
}

func TestJournalUndo(t *testing.T) {
	root := t.TempDir()
	v := Vault{Dir: filepath.Join(root, "vault")}
	created, replaced, removed := filepath.Join(root, "new.json"), filepath.Join(root, "old.json"), filepath.Join(root, "gone.json")
	put(t, replaced, "before")
	put(t, removed, "gone-bytes")

	now := time.Date(2026, 9, 20, 10, 30, 0, 123e6, time.Local)
	j, err := v.NewJournal("move", now)
	if err != nil {
		t.Fatal(err)
	}
	if j.ID != "20260920-103000-123" {
		t.Errorf("id = %q, want v2's yyyyMMdd-HHmmss-fff", j.ID)
	}
	must(t, j.Created(created))
	put(t, created, "new")
	must(t, j.Removed(replaced))
	put(t, replaced, "after")
	must(t, j.Removed(removed))
	os.Remove(removed)
	must(t, j.Save("Moved 2 chats"))

	second, _ := v.NewJournal("copy", now)
	if second.ID <= j.ID {
		t.Errorf("same-millisecond journal got id %q after %q", second.ID, j.ID)
	}
	must(t, second.Save("nothing"))
	if _, err := os.Stat(second.Dir); err == nil {
		t.Error("an empty journal should not be saved")
	}

	js, err := v.Journals()
	if err != nil || len(js) != 1 || js[0].Summary != "Moved 2 chats" || js[0].Action != "move" || len(js[0].Entries) != 3 || js[0].Undone || !js[0].At.Equal(now) {
		t.Fatalf("journals = %+v, %v", js, err)
	}
	must(t, v.Undo(js[0]))
	if _, err := os.Stat(created); err == nil {
		t.Error("undo should remove the created file")
	}
	if read(t, replaced) != "before" || read(t, removed) != "gone-bytes" {
		t.Error("undo should restore the old bytes")
	}
	if js, _ := v.Journals(); !js[0].Undone {
		t.Error("undone journal not marked")
	}
}

// A journal exactly as v2 (Windows PowerShell 5.1 ConvertTo-Json) wrote it.
func TestUndoV2Journal(t *testing.T) {
	root := t.TempDir()
	v := Vault{Dir: filepath.Join(root, "vault")}
	dir := filepath.Join(v.Dir, "_journal", "20260920-103000-123")
	target, source := filepath.Join(root, "persDir", "local_c2.json"), filepath.Join(root, "workDir", "local_c2.json")
	backup := filepath.Join(dir, "1-local_c2.json")
	put(t, target, `{"moved":true}`)
	put(t, backup, `{"original":"bytes"}`)
	esc := func(p string) string { return strings.ReplaceAll(p, `\`, `\\`) }
	put(t, filepath.Join(dir, "journal.json"), "{\r\n    \"action\":  \"move\",\r\n    \"at\":  \"2026-09-20T10:30:00.1234567+03:30\",\r\n    \"summary\":  \"Moved 1 chat(s) from work to personal\",\r\n    \"entries\":  [\r\n                    {\r\n                        \"Op\":  \"created\",\r\n                        \"Path\":  \""+esc(target)+"\",\r\n                        \"Backup\":  \"\"\r\n                    },\r\n                    {\r\n                        \"Op\":  \"removed\",\r\n                        \"Path\":  \""+esc(source)+"\",\r\n                        \"Backup\":  \""+esc(backup)+"\"\r\n                    }\r\n                ]\r\n}")

	js, err := v.Journals()
	if err != nil || len(js) != 1 || js[0].At.IsZero() || len(js[0].Entries) != 2 {
		t.Fatalf("v2 journal not read: %+v %v", js, err)
	}
	must(t, v.Undo(js[0]))
	if _, err := os.Stat(target); err == nil || read(t, source) != `{"original":"bytes"}` {
		t.Error("v2 journal not undone byte for byte")
	}
}

// Folder names are local time, which runs back an hour when daylight saving ends; v2 also left
// journals of changes that wrote nothing (found on a real machine), which Undo must pass over.
func TestJournalsNewestByTimeSkipsEmpty(t *testing.T) {
	v := Vault{Dir: t.TempDir()}
	for id, at := range map[string]string{
		"20261101-013000-000": "2026-11-01T01:30:00-04:00", // before the clocks went back
		"20261101-011000-000": "2026-11-01T01:10:00-05:00", // after: 40 minutes later
	} {
		put(t, filepath.Join(v.Dir, "_journal", id, "journal.json"),
			`{"action":"copy","at":"`+at+`","summary":"`+id+`","entries":[{"Op":"created","Path":"x","Backup":""}]}`)
	}
	put(t, filepath.Join(v.Dir, "_journal", "20261102-090000-000", "journal.json"),
		`{"action":"merge","at":"2026-11-02T09:00:00-05:00","summary":"empty","entries":[]}`)
	js, err := v.Journals()
	must(t, err)
	var got []string
	for _, j := range js {
		got = append(got, j.Summary)
	}
	if strings.Join(got, ",") != "20261101-011000-000,20261101-013000-000" {
		t.Errorf("journals = %v, want the later change first and no empty one", got)
	}
}

func TestUndoKeepsJournalOpenOnFailure(t *testing.T) {
	v := Vault{Dir: t.TempDir()}
	j, _ := v.NewJournal("copy", time.Now())
	j.Entries = []Entry{{Op: OpRemoved, Path: filepath.Join(v.Dir, "x"), Backup: filepath.Join(v.Dir, "missing")}}
	must(t, j.Save("s"))
	if v.Undo(*j) == nil {
		t.Fatal("a missing backup should fail")
	}
	if js, _ := v.Journals(); js[0].Undone {
		t.Error("a failed undo must not be marked done")
	}
}

func TestSettings(t *testing.T) {
	v := Vault{Dir: t.TempDir()}
	s, err := v.Settings()
	if err != nil || !reflect.DeepEqual(s, DefaultSettings()) {
		t.Fatalf("defaults = %+v, %v", s, err)
	}
	// Keys this version does not use (an earlier one's language, a later one's anything) stay.
	put(t, filepath.Join(v.Dir, "settings.json"), `{"lang":"fa","future":{"a":[1,2]},"rtl":"off"}`)
	s, err = v.Settings()
	if err != nil || s.RTL != "off" || s.Theme != "auto" || !s.UpdateCheck {
		t.Fatalf("read = %+v, %v", s, err)
	}
	s.Onboarded = true
	must(t, v.SaveSettings(s))
	text := read(t, filepath.Join(v.Dir, "settings.json"))
	if !strings.Contains(text, `"future": {`) || !strings.Contains(text, `"lang": "fa"`) || !strings.Contains(text, `"onboarded": true`) {
		t.Errorf("unknown keys lost or setting not saved:\n%s", text)
	}
	if again, _ := v.Settings(); !again.Onboarded || again.RTL != "off" {
		t.Error("settings did not round trip")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
