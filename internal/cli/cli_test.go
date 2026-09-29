package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestProfiles(t *testing.T) {
	f := newFixture(t)
	f.signIn(work, "work")
	f.ok("save", "work")
	if f.read(filepath.Join(f.vault, "work", "_account")) != work || f.read(filepath.Join(f.vault, "_current_profile")) != "work" {
		t.Fatal("save should record the account and the profile in use")
	}
	f.ok("add", "personal")
	if exist(filepath.Join(f.data, "config.json")) {
		t.Fatal("add should leave Desktop signed out")
	}
	if r := f.ok("list"); !strings.Contains(r.out, "personal") || !strings.Contains(r.out, "not saved yet") {
		t.Errorf("list should show the profile in use that is not saved yet:\n%s", r.out)
	}

	f.signIn(pers, "personal")
	f.ok("switch", "work")
	if f.read(filepath.Join(f.vault, "personal", "_account")) != pers || !strings.Contains(f.read(filepath.Join(f.data, "config.json")), work) {
		t.Fatal("switch should save the account it leaves and restore work")
	}
	if r := f.ok("switch", "WORK"); !strings.Contains(r.out, "already") {
		t.Errorf("switching to the profile in use is fine: %q", r.out)
	}

	f.put(filepath.Join(f.data, "config.json"), `{"lastKnownAccountUuid":"33333333-3333-4333-8333-333333333333"}`)
	before := hash(f.data)
	r := f.run("switch", "personal")
	f.want(r, exitRefused, "switch after signing in by hand")
	if hash(f.data) != before || !strings.Contains(r.err, "✗") || !strings.Contains(r.err, "different account") {
		t.Errorf("a refused switch changes nothing and says why:\n%s", r.err)
	}
	f.signIn(work, "work")

	f.want(f.run("switch", "nobody"), exitNotFound, "switch nobody")
	f.want(f.run("save", "_hidden"), exitUsage, "save _hidden")
	f.want(f.run("remove", "work"), exitRefused, "remove the profile in use")
	r = f.run("remove", "personal")
	f.want(r, exitRefused, "remove without --yes and without a terminal")
	if !strings.Contains(r.err, "--yes") || !exist(filepath.Join(f.vault, "personal")) {
		t.Errorf("an unconfirmed remove must refuse with a hint and keep the profile:\n%s", r.err)
	}

	f.ok("rename", "work", "کار")
	if !exist(filepath.Join(f.vault, "کار", "config.json")) {
		t.Fatal("profiles can take Persian names")
	}
	f.ok("rename", "کار", "work")
	f.ok("remove", "personal", "--yes")
	if exist(filepath.Join(f.vault, "personal")) {
		t.Error("remove --yes should forget the profile")
	}
}

func TestListJSON(t *testing.T) {
	f := chatFixture(t)
	v := decode[map[string]any](t, f.ok("list", "--json"))
	if got := keys(v); !slices.Equal(got, []string{"account", "current", "dataDir", "desktopRunning", "lists", "profiles"}) {
		t.Fatalf("keys = %v", got)
	}
	profiles := v["profiles"].([]any)
	first := profiles[0].(map[string]any)
	if len(profiles) != 2 || !slices.Equal(keys(first), []string{"account", "chats", "current", "name", "notSavedYet", "saved", "signedIn"}) {
		t.Fatalf("profiles = %v", profiles)
	}
	lists := v["lists"].([]any)
	if len(lists) != 2 || v["current"] != "work" || v["account"] != work {
		t.Errorf("list = %v", v)
	}
}

func TestChats(t *testing.T) {
	f := chatFixture(t)
	r := f.ok("chats", "work")
	if i, j := strings.Index(r.out, "Design review"), strings.Index(r.out, "Release notes"); i < 0 || j < i {
		t.Errorf("chats should list newest first:\n%s", r.out)
	}
	r = f.ok("chats")
	if !strings.Contains(r.out, persianTitle) || !strings.Contains(r.out, "Design review") {
		t.Errorf("chats without a list shows every list:\n%s", r.out)
	}
	if r = f.ok("chats", "--limit", "1"); !strings.Contains(r.out, "Showing 1 of 3 chats") || strings.Contains(r.out, "Release notes") {
		t.Errorf("--limit:\n%s", r.out)
	}

	chats := decode[[]map[string]any](t, f.ok("chats", "--json"))
	if len(chats) != 3 || chats[0]["title"] != persianTitle || chats[0]["id"] != "local_c3" {
		t.Fatalf("chats = %v", chats)
	}
	want := []string{"account", "archived", "created", "cwd", "hasHistory", "id", "lastUsed", "list", "model", "org", "prior", "project", "session", "title"}
	if got := keys(chats[0]); !slices.Equal(got, want) {
		t.Errorf("chat keys = %v, want %v", got, want)
	}
	f.want(f.run("chats", "nowhere"), exitNotFound, "chats nowhere")
}

func TestCopyMoveUndo(t *testing.T) {
	f := chatFixture(t)
	workDir, persDir := f.listDir(work), f.listDir(pers)
	workBefore := hash(workDir)

	f.want(f.run("copy", "--from", "work", "--to", "personal", "c1"), exitRefused, "copy without --yes")
	if exist(filepath.Join(persDir, "local_c1.json")) {
		t.Fatal("an unconfirmed copy must change nothing")
	}
	r := f.ok("copy", "--from", "work", "--to", "personal", "c1", "--yes")
	if !exist(filepath.Join(persDir, "local_c1.json")) || hash(workDir) != workBefore {
		t.Fatal("copy puts the chat in the target and leaves the source alone")
	}
	if !strings.Contains(r.err, "1 new · 0 updated · 0 already newer there") || !strings.Contains(r.out, "✓ Copied 1 chat") {
		t.Errorf("copy should show the plan and the result:\n%s%s", r.err, r.out)
	}
	f.ok("undo", "--yes")
	if exist(filepath.Join(persDir, "local_c1.json")) {
		t.Fatal("undo removes the copied chat")
	}

	f.ok("move", "--from", "work", "--to", "personal", "--chat", "c2", "--yes")
	if !exist(filepath.Join(persDir, "local_c2.json")) || exist(filepath.Join(workDir, "local_c2.json")) {
		t.Fatal("move puts the chat in the target and removes it from the source")
	}
	f.ok("undo", "--yes")
	if hash(workDir) != workBefore || exist(filepath.Join(persDir, "local_c2.json")) {
		t.Fatal("undo of a move restores the source byte for byte")
	}

	r = f.run("move", "--from", "work", "--to", "personal", "nothing-like-this", "--yes")
	f.want(r, exitNotFound, "move an unknown chat")
	if hash(workDir) != workBefore || !strings.Contains(r.err, "nothing-like-this") {
		t.Errorf("an unknown chat id changes nothing:\n%s", r.err)
	}
	f.want(f.run("copy", "--from", "work", "c1"), exitUsage, "copy without --to")

	v := decode[map[string]any](t, f.ok("copy", "--from", "work", "--to", "personal", "c1", "--yes", "--json"))
	if got := keys(v); !slices.Equal(got, []string{"created", "from", "kind", "removed", "skipped", "summary", "to", "updated"}) || v["created"] != 1.0 {
		t.Errorf("copy --json = %v", v)
	}
	r = f.ok("copy", "--from", "work", "--to", "personal", "c1", "--yes")
	if !strings.Contains(r.out, "Nothing to do") {
		t.Errorf("copying onto a copy as new changes nothing:\n%s", r.out)
	}

	history := decode[[]map[string]any](t, f.ok("history", "--json"))
	if len(history) != 3 || history[0]["action"] != "copy" || history[1]["undone"] != true {
		t.Errorf("history = %v", history)
	}
	if r := f.ok("log"); !strings.Contains(r.out, "undone") {
		t.Errorf("history marks undone changes:\n%s", r.out)
	}
}

func TestMerge(t *testing.T) {
	f := chatFixture(t)
	workDir := f.listDir(work)
	before := hash(workDir)
	f.card(pers, "c1", s1, "Design review (older copy)", time.Now().Add(-100*time.Hour))
	f.ok("merge", "--to", "work", "--yes")
	if !exist(filepath.Join(workDir, "local_c3.json")) || strings.Contains(f.read(filepath.Join(workDir, "local_c1.json")), "older copy") {
		t.Fatal("merge copies what the target is missing and never an older copy")
	}
	f.ok("undo", "--yes")
	if hash(workDir) != before {
		t.Fatal("undo of a merge restores the target")
	}
	f.want(f.run("merge"), exitUsage, "merge without --to")

	older := filepath.Join(f.listDir(pers), "local_c1.json")
	r := f.ok("copy", "--from", "work", "--to", "personal", "c1", "--yes")
	if !strings.Contains(r.err, "0 new · 1 updated") || strings.Contains(f.read(older), "older copy") {
		t.Fatalf("copy brings an older copy in the target up to date:\n%s", r.err)
	}
	f.ok("undo", "--yes")
	if !strings.Contains(f.read(older), "older copy") {
		t.Error("undo puts the older copy back")
	}
}

func TestRescue(t *testing.T) {
	f := chatFixture(t)
	f.transcript(o1, persianTitle+" ۲", "fix the translation bug", time.Now().Add(-48*time.Hour))
	f.transcript(o2, "", "build the login page", time.Now().Add(-24*time.Hour))
	r := f.ok("rescue")
	if !strings.Contains(r.out, "2 chats are on disk but in no chat list") || strings.Contains(r.out, "Design review") {
		t.Errorf("rescue lists only the lost chats:\n%s", r.out)
	}
	lost := decode[map[string]any](t, f.ok("chats", "--lost", "--json"))
	if len(lost["lost"].([]any)) != 2 {
		t.Fatalf("chats --lost = %v", lost)
	}

	before := len(cards(t, f.listDir(work)))
	f.ok("rescue", "--to", "work", "--all", "--yes")
	if got := len(cards(t, f.listDir(work))); got != before+2 {
		t.Fatalf("rescue --all wrote %d cards, want 2", got-before)
	}
	if r := f.ok("chats", "--lost"); !strings.Contains(r.out, "Every chat on disk is in a chat list") {
		t.Errorf("nothing is left to rescue:\n%s", r.out)
	}
	f.ok("undo", "--yes")
	if got := len(cards(t, f.listDir(work))); got != before {
		t.Errorf("undo takes the recovery back: %d cards", got)
	}
	f.want(f.run("rescue", "ffffffff", "--yes"), exitNotFound, "rescue an unknown id")
}

func cards(t *testing.T, dir string) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(dir, "local_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestExport(t *testing.T) {
	f := chatFixture(t)
	out := filepath.Join(t.TempDir(), "chat.md")
	f.ok("export", "c3", "--format", "md", "-o", out)
	if md := f.read(out); !strings.Contains(md, persianTitle) || !strings.Contains(md, "hello") {
		t.Errorf("markdown export:\n%s", md)
	}
	if r := f.ok("export", "c1", "--format", "md", "-o", "-"); !strings.Contains(r.out, "Design review") {
		t.Errorf("-o - writes to stdout:\n%s", r.out)
	}

	t.Chdir(t.TempDir())
	f.ok("export", "c1")
	f.ok("export", "local_c1")
	if !exist("Design review.html") || !exist("Design review (2).html") {
		t.Error("export names the file after the chat and never overwrites one")
	}
	f.want(f.run("export", "c1", "--format", "pdf"), exitUsage, "export as pdf")
	f.want(f.run("export", "zzz"), exitNotFound, "export an unknown chat")
	f.want(f.run("export", "c"), exitUsage, "export an ambiguous id")
}

func TestDoctor(t *testing.T) {
	f := chatFixture(t)
	checks := decode[[]map[string]any](t, f.ok("doctor", "--json"))
	if len(checks) == 0 || checks[0]["level"] != "ok" || checks[0]["title"] == "" {
		t.Fatalf("doctor = %v", checks)
	}
	if r := f.ok("doctor"); !strings.Contains(r.out, "✓") {
		t.Errorf("doctor shows icons:\n%s", r.out)
	}
	linked := filepath.Join(f.data, "claude-code-sessions", "44444444-4444-4444-8444-444444444444", org)
	os.MkdirAll(filepath.Dir(linked), 0o700)
	makeLink(t, f.listDir(pers), linked)
	r := f.run("doctor")
	f.want(r, exitError, "doctor with a linked chat list")
	if !strings.Contains(r.out, "✗") || !strings.Contains(r.out, "junction") {
		t.Errorf("doctor flags a linked chat list:\n%s", r.out)
	}
}

func TestUsage(t *testing.T) {
	f := chatFixture(t)
	v := decode[map[string]any](t, f.ok("usage", "--json"))
	if v["totalTokens"] != 3*11110.0 || v["sessions"] != 3.0 || v["byModel"].(map[string]any)["claude-opus-5"] == nil {
		t.Errorf("usage --json = %v", v)
	}
	if r := f.ok("usage", "--days", "7"); !strings.Contains(r.out, "claude-opus-5") || !strings.Contains(r.out, "last 7 days") {
		t.Errorf("usage:\n%s", r.out)
	}
}

func TestConfig(t *testing.T) {
	f := newFixture(t)
	v := decode[map[string]any](t, f.ok("config", "--json"))
	want := map[string]any{"lang": "auto", "theme": "auto", "rtl": "auto", "persianDigits": true, "jalali": true, "updateCheck": true}
	if len(v) != len(want) || v["lang"] != "auto" || v["updateCheck"] != true {
		t.Fatalf("config = %v, want %v", v, want)
	}
	f.ok("config", "set", "lang", "fa")
	f.ok("config", "set", "theme", "light")
	f.ok("config", "set", "update-check", "off")
	if r := f.ok("config", "get", "lang"); r.out != "fa\n" {
		t.Errorf("config get lang = %q", r.out)
	}
	s := f.read(filepath.Join(f.vault, "settings.json"))
	if !strings.Contains(s, `"theme":"nyxon-light"`) && !strings.Contains(s, `"theme": "nyxon-light"`) {
		t.Errorf("settings.json keeps the long theme name: %s", s)
	}
	if r := f.ok("config", "list"); !regexp.MustCompile(`[آ-ی]`).MatchString(r.out) {
		t.Errorf("a saved language applies to the next run:\n%s", r.out)
	}
	f.ok("config", "set", "lang", "auto")
	f.want(f.run("config", "set", "theme", "neon"), exitUsage, "config set theme neon")
	f.want(f.run("config", "get", "colour"), exitUsage, "config get colour")

	broken := filepath.Join(f.vault, "settings.json")
	f.put(broken, `{"lang": "fa",`)
	f.want(f.run("config", "set", "jalali", "off"), exitError, "config set over a broken settings file")
	if f.read(broken) != `{"lang": "fa",` {
		t.Error("a settings file that cannot be read must not be replaced by the defaults")
	}
}

func TestVersion(t *testing.T) {
	f := newFixture(t)
	if r := f.ok("version", "--short"); r.out != "3.0.0\n" {
		t.Errorf("version --short = %q", r.out)
	}
	v := decode[map[string]any](t, f.ok("version", "--json"))
	if got := keys(v); !slices.Equal(got, []string{"arch", "commit", "date", "go", "install", "os", "version"}) || v["commit"] != "abc1234" {
		t.Errorf("version --json = %v", v)
	}
	if r := f.ok("version"); !strings.Contains(r.out, "Claude Switcher 3.0.0") || !strings.Contains(r.out, "abc1234") {
		t.Errorf("version:\n%s", r.out)
	}
}

func TestPersian(t *testing.T) {
	f := chatFixture(t)
	for _, args := range [][]string{{"list"}, {"chats", "--limit", "2"}, {"doctor"}, {"usage"}} {
		r := f.ok(append(args, "--lang", "fa")...)
		if !utf8.ValidString(r.out) || !strings.ContainsAny(r.out, "۰۱۲۳۴۵۶۷۸۹") || !regexp.MustCompile(`[آ-ی]`).MatchString(r.out) {
			t.Errorf("%v --lang fa should be Persian with Persian digits:\n%s", args, r.out)
		}
	}
	r := f.run("switch", "nobody", "--lang", "fa")
	if !strings.Contains(r.err, "«nobody»") || r.code != exitNotFound {
		t.Errorf("errors are Persian too:\n%s", r.err)
	}
}

func TestUsageErrors(t *testing.T) {
	f := newFixture(t)
	for _, args := range [][]string{{"nonsense"}, {"chats", "a", "b"}, {"list", "--lang", "xx"}, {"list", "--nope"}, {"switch"}} {
		r := f.run(args...)
		f.want(r, exitUsage, args...)
		if !strings.HasPrefix(r.err, "✗ ") || !strings.Contains(r.err, "--help") {
			t.Errorf("%v: a usage error says what is wrong and where to look:\n%s", args, r.err)
		}
	}
	r := f.ok("help")
	if !strings.Contains(r.out, "switch <profile>") || !strings.Contains(r.out, "rescue") || !strings.Contains(r.out, "5 cancelled") {
		t.Errorf("help lists the commands and the exit codes:\n%s", r.out)
	}
}

func TestWritesWaitForDesktop(t *testing.T) {
	f := chatFixture(t)
	d := &fakeDesktop{running: true, closeAt: 2}
	withDesktop(t, d)
	r := f.ok("switch", "personal")
	if !strings.Contains(r.err, "tray") || !strings.Contains(r.err, "Claude Desktop is closed") || d.launches != 1 ||
		!strings.Contains(f.read(filepath.Join(f.data, "config.json")), pers) {
		t.Fatalf("switch should wait for Desktop, switch and start it again (launches %d):\n%s", d.launches, r.err)
	}
	f.ok("copy", "--from", "work", "--to", "personal", "c1", "--yes")
	if d.launches != 1 {
		t.Error("a chat change only starts Desktop again when it was running")
	}
	f.ok("switch", "work", "--no-launch")
	if d.launches != 1 {
		t.Error("--no-launch leaves Desktop closed")
	}
	f.ok("switch", "personal")
	if d.launches != 2 {
		t.Error("switch always starts Desktop")
	}
}

func TestUnattendedWriteRefusesWhileDesktopRuns(t *testing.T) {
	f := chatFixture(t)
	d := &fakeDesktop{running: true}
	withDesktop(t, d)
	old := unattendedWait
	unattendedWait = 100 * time.Millisecond
	t.Cleanup(func() { unattendedWait = old })

	before := hash(f.data)
	r := f.run("switch", "personal")
	f.want(r, exitRefused, "switch while Desktop stays open")
	if hash(f.data) != before || !strings.Contains(r.err, "--force-quit") || d.launches != 0 {
		t.Fatalf("nothing changes and the hint names --force-quit (launches %d):\n%s", d.launches, r.err)
	}
	f.ok("switch", "personal", "--force-quit")
	if d.forced != 1 {
		t.Error("--force-quit closes Desktop")
	}
}
