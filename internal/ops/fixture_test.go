package ops

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

const (
	work  = "11111111-1111-4111-8111-111111111111"
	pers  = "22222222-2222-4222-8222-222222222222"
	third = "33333333-3333-4333-8333-333333333333"
	org   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

var proj = filepath.FromSlash("/work/proj")

// fakeDesktop stands in for Claude Desktop. It closes on its own after closeAfter polls.
type fakeDesktop struct {
	running    bool
	closeAfter int
	polls      int
	helpers    int
	launches   int
}

func (d *fakeDesktop) Running() (bool, error) {
	d.polls++
	if d.closeAfter > 0 && d.polls > d.closeAfter {
		d.running = false
	}
	return d.running, nil
}
func (d *fakeDesktop) Quit() error      { return nil }
func (d *fakeDesktop) ForceQuit() error { return nil }
func (d *fakeDesktop) Launch() error    { d.launches++; return nil }
func (d *fakeDesktop) StopHelpers()     { d.helpers++ }

var _ platform.Desktop = (*fakeDesktop)(nil)

// fixture is a throwaway Desktop data folder, vault and projects folder. Transcripts are fakes:
// metas maps a session id to what the transcript reader would return.
type fixture struct {
	*Env
	t     *testing.T
	root  string
	data  string
	desk  *fakeDesktop
	metas map[string]transcript.Meta
}

func newFixture(t *testing.T) *fixture {
	root := t.TempDir()
	f := &fixture{t: t, root: root, data: filepath.Join(root, "Claude"), desk: &fakeDesktop{}, metas: map[string]transcript.Meta{}}
	env, err := Open(Options{DataDir: f.data, VaultDir: filepath.Join(root, "vault"), ProjectsDir: filepath.Join(root, "claude", "projects")})
	if err != nil {
		t.Fatal(err)
	}
	f.Env = env
	env.Desktop = f.desk
	env.Index = func(string) (map[string]string, error) {
		index := map[string]string{}
		for s := range f.metas {
			index[s] = f.transcriptPath(s)
		}
		return index, nil
	}
	env.QuickMeta = func(path string) (transcript.Meta, error) {
		return f.metas[strings.TrimSuffix(filepath.Base(path), ".jsonl")], nil
	}
	env.ReadMeta = env.QuickMeta
	os.MkdirAll(env.Vault.Dir, 0o700)
	return f
}

func (f *fixture) transcriptPath(session string) string {
	return filepath.Join(f.Projects, "proj", session+".jsonl")
}

// transcript registers a fake transcript in proj.
func (f *fixture) transcript(session, title, prompt string, end time.Time) {
	f.metas[session] = transcript.Meta{Session: session, Path: f.transcriptPath(session), Cwd: proj, Title: title,
		FirstPrompt: prompt, Model: "claude-opus-5", Start: end.Add(-5 * time.Minute), End: end}
}

func (f *fixture) put(path, text string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) read(path string) string {
	f.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func (f *fixture) exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// listDir is an account's chat list folder.
func (f *fixture) listDir(account string) string {
	return filepath.Join(f.data, "claude-code-sessions", account, org)
}

// card writes a chat card and returns its path. extra is raw JSON appended to the object.
func (f *fixture) card(account, id, session, title string, last int64, extra string) string {
	path := filepath.Join(f.listDir(account), "local_"+id+".json")
	f.put(path, fmt.Sprintf(`{"sessionId":"local_%s","cliSessionId":%s,"cwd":%s,"title":%s,"lastActivityAt":%d%s}`,
		id, str(session), str(proj), str(title), last, extra))
	return path
}

func str(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// signIn fills Desktop's login items as account who.
func (f *fixture) signIn(account, who string) {
	f.put(filepath.Join(f.data, "config.json"), `{"lastKnownAccountUuid":"`+account+`","userThemeMode":"dark"}`)
	for _, name := range []string{"Preferences", "DIPS", "DIPS-wal", "SharedStorage", "ant-did", "buddy-tokens.json"} {
		f.put(filepath.Join(f.data, name), who+"-"+name)
	}
	for _, name := range []string{"Local Storage/leveldb/000003.log", "Session Storage/000003.log", "Network/Cookies", "IndexedDB/https_claude.ai_0.indexeddb.leveldb/LOG"} {
		f.put(filepath.Join(f.data, filepath.FromSlash(name)), who+"-"+name)
	}
}

// hash fingerprints every file under dir, names and contents.
func (f *fixture) hash(dir string) string {
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
	return strings.Join(lines, "\n")
}

// login fingerprints Desktop's login items only.
func (f *fixture) login() string {
	var parts []string
	for _, it := range f.Items {
		p := filepath.Join(f.data, it)
		if st, err := os.Stat(p); err != nil {
			parts = append(parts, it+"=-")
		} else if st.IsDir() {
			parts = append(parts, it+"/"+f.hash(p))
		} else {
			parts = append(parts, it+"="+f.read(p))
		}
	}
	return strings.Join(parts, ";")
}

func (f *fixture) list(ref string) List {
	f.t.Helper()
	l, err := f.FindList(ref)
	if err != nil {
		f.t.Fatalf("list %q: %v", ref, err)
	}
	return l
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantCode(t *testing.T, err error, c Code) {
	t.Helper()
	if !Is(err, c) {
		t.Fatalf("got %v, want %s", err, c)
	}
}
