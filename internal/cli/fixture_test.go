package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
)

const (
	work = "11111111-1111-4111-8111-111111111111"
	pers = "22222222-2222-4222-8222-222222222222"
	org  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	s1   = "10000000-0000-4000-8000-000000000001"
	s2   = "10000000-0000-4000-8000-000000000002"
	s3   = "10000000-0000-4000-8000-000000000003"
	o1   = "20000000-0000-4000-8000-000000000001"
	o2   = "20000000-0000-4000-8000-000000000002"

	persianTitle = "بازبینی طراحی"
)

var proj = filepath.FromSlash("/work/proj")

// fixture is a throwaway Claude Desktop data folder, vault and projects folder, driven through
// the hidden --data-dir, --vault-dir and --projects-dir flags.
type fixture struct {
	t                     *testing.T
	data, vault, projects string
}

func newFixture(t *testing.T) *fixture {
	t.Setenv("CLAUDE_SWITCHER_LANG", "en")
	t.Setenv("CLAUDE_SWITCHER_NO_UPDATE_CHECK", "1")
	root := t.TempDir()
	return &fixture{t: t, data: filepath.Join(root, "Claude"), vault: filepath.Join(root, "vault"), projects: filepath.Join(root, "claude", "projects")}
}

type result struct {
	out, err string
	code     int
}

func (f *fixture) run(args ...string) result {
	f.t.Helper()
	var out, errOut bytes.Buffer
	args = append(args, "--data-dir", f.data, "--vault-dir", f.vault, "--projects-dir", f.projects)
	code := run(context.Background(), Build{Version: "3.0.0", Commit: "abc1234", Date: "2026-09-29T10:00:00Z"},
		args, strings.NewReader(""), &out, &errOut)
	return result{out.String(), errOut.String(), code}
}

// term runs a command as if stdout were a terminal width cells wide that leaves Persian to the
// app (rtl.App), as Windows Terminal does.
func (f *fixture) term(width int, args ...string) result {
	f.t.Helper()
	var out, errOut bytes.Buffer
	args = append(args, "--data-dir", f.data, "--vault-dir", f.vault, "--projects-dir", f.projects)
	a := newApp(Build{Version: "3.0.0", Commit: "abc1234", Date: "2026-09-29T10:00:00Z"}, args, strings.NewReader(""), &out, &errOut)
	a.tty, a.mode, a.width, a.mirror = true, rtl.App, width, i18n.RTL()
	code := a.execute(context.Background(), args)
	return result{out.String(), errOut.String(), code}
}

// ok runs a command that must succeed.
func (f *fixture) ok(args ...string) result {
	f.t.Helper()
	r := f.run(args...)
	if r.code != 0 {
		f.t.Fatalf("%v: exit %d\n%s%s", args, r.code, r.out, r.err)
	}
	return r
}

func (f *fixture) want(r result, code int, args ...string) {
	f.t.Helper()
	if r.code != code {
		f.t.Fatalf("%v: exit %d, want %d\n%s%s", args, r.code, code, r.out, r.err)
	}
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
	data, _ := os.ReadFile(path)
	return string(data)
}

func exist(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// signIn fills Desktop's login items as account who.
func (f *fixture) signIn(account, who string) {
	f.put(filepath.Join(f.data, "config.json"), `{"lastKnownAccountUuid":"`+account+`"}`)
	for _, name := range []string{"Preferences", "DIPS", "ant-did", filepath.Join("Network", "Cookies")} {
		f.put(filepath.Join(f.data, name), who+"-"+name)
	}
}

func (f *fixture) listDir(account string) string {
	return filepath.Join(f.data, "claude-code-sessions", account, org)
}

func (f *fixture) card(account, id, session, title string, last time.Time) string {
	path := filepath.Join(f.listDir(account), "local_"+id+".json")
	f.put(path, fmt.Sprintf(`{"sessionId":"local_%s","cliSessionId":%s,"cwd":%s,"title":%s,"createdAt":%d,"lastActivityAt":%d}`,
		id, str(session), str(proj), str(title), last.Add(-time.Hour).UnixMilli(), last.UnixMilli()))
	return path
}

func str(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// transcript writes a small Claude Code transcript: a prompt, a reply with token usage and,
// when title is set, a custom title.
func (f *fixture) transcript(session, title, prompt string, at time.Time) {
	ts := func(d time.Duration) string { return at.Add(d).UTC().Format(time.RFC3339Nano) }
	lines := []string{
		`{"type":"user","uuid":"u-` + session + `","timestamp":"` + ts(-time.Minute) + `","cwd":` + str(proj) + `,"sessionId":"` + session +
			`","message":{"role":"user","content":` + str(prompt) + `}}`,
		`{"type":"assistant","uuid":"a-` + session + `","timestamp":"` + ts(0) + `","cwd":` + str(proj) + `,"sessionId":"` + session +
			`","message":{"id":"m-` + session + `","model":"claude-opus-5","role":"assistant","content":[{"type":"text","text":"done"}],` +
			`"usage":{"input_tokens":10,"output_tokens":100,"cache_creation_input_tokens":1000,"cache_read_input_tokens":10000}}}`,
	}
	if title != "" {
		lines = append(lines, `{"type":"custom-title","customTitle":`+str(title)+`,"sessionId":"`+session+`"}`)
	}
	f.put(filepath.Join(f.projects, "C--work-proj", session+".jsonl"), strings.Join(lines, "\n")+"\n")
}

// hash fingerprints every file under dir, names and contents.
func hash(dir string) string {
	var lines []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(path)
			rel, _ := filepath.Rel(dir, path)
			lines = append(lines, fmt.Sprintf("%s=%x", rel, sha256.Sum256(data)))
		}
		return nil
	})
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

// chatFixture has profiles work (signed in, in use) and personal, set up through the commands
// themselves, and three chats: c1 and c2 in work, c3 with a Persian title in personal.
func chatFixture(t *testing.T) *fixture {
	f := newFixture(t)
	f.signIn(work, "work")
	f.ok("save", "work")
	f.ok("add", "personal")
	f.signIn(pers, "personal")
	f.ok("switch", "work")
	now := time.Now()
	f.card(work, "c1", s1, "Design review", now.Add(-time.Hour))
	f.card(work, "c2", s2, "Release notes", now.Add(-2*time.Hour))
	f.card(pers, "c3", s3, persianTitle, now.Add(-time.Minute))
	for _, s := range []string{s1, s2, s3} {
		f.transcript(s, "", "hello", now.Add(-time.Minute))
	}
	return f
}

func decode[T any](t *testing.T, r result) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(r.out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.out)
	}
	return v
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// fakeDesktop stands in for a running Claude Desktop. It closes on the poll closeAt.
type fakeDesktop struct {
	running                          bool
	closeAt, polls, launches, forced int
}

func (d *fakeDesktop) Running() (bool, error) {
	d.polls++
	if d.closeAt > 0 && d.polls >= d.closeAt {
		d.running = false
	}
	return d.running, nil
}
func (d *fakeDesktop) Quit() error      { return platform.ErrManualQuit }
func (d *fakeDesktop) ForceQuit() error { d.forced++; d.running = false; return nil }
func (d *fakeDesktop) Launch() error    { d.launches++; return nil }
func (d *fakeDesktop) StopHelpers()     {}

// withDesktop makes the commands see d as an installed Claude Desktop.
func withDesktop(t *testing.T, d *fakeDesktop) {
	old := openEnv
	openEnv = func(o ops.Options) (*ops.Env, error) {
		env, err := old(o)
		if err == nil {
			env.Desktop, env.Install.Kind = d, platform.MSIX
		}
		return env, err
	}
	t.Cleanup(func() { openEnv = old })
}
