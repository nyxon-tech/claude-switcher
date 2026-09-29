package ops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
)

// Level is how much a check needs the user.
type Level string

const (
	OK   Level = "ok"
	Info Level = "info"
	Warn Level = "warn"
	Fail Level = "fail"
)

// Check is one line of the doctor's report, in the current language.
type Check struct {
	Level  Level
	Title  string
	Detail string // "" when the title says it all
	Fix    string // what to do about it, "" when nothing
}

// Doctor checks the setup: the install, the accounts, every chat list, lost chats, how long
// Claude Code keeps chats, and on Windows the Cowork VM. It changes nothing.
func (e *Env) Doctor() []Check {
	out := e.installChecks()
	out = append(out, platformChecks(e)...)
	st, err := e.Status()
	if err != nil {
		return append(out, Check{Level: Fail, Title: i18n.T("ops.doctor.read-failed"), Detail: err.Error()})
	}
	out = append(out, e.accountChecks(st)...)
	out = append(out, e.listChecks(st)...)
	return append(out, e.retentionChecks()...)
}

func (e *Env) installChecks() []Check {
	kind := i18n.T("ops.kind." + string(e.Install.Kind))
	out := []Check{{Level: OK, Title: i18n.T("ops.doctor.install", "kind", kind), Detail: e.Install.DataDir}}
	if !e.signedIn() {
		out[0] = Check{Level: Warn, Title: i18n.T("ops.doctor.no-login", "kind", kind), Detail: e.Install.DataDir, Fix: i18n.T("ops.doctor.no-login.fix")}
	}
	if e.Install.Kind == platform.Custom {
		return out // nothing runs from a folder given by hand
	}
	switch running, err := e.Desktop.Running(); {
	case err != nil:
		out = append(out, Check{Level: Warn, Title: i18n.T("ops.doctor.running-unknown"), Detail: err.Error()})
	case running:
		out = append(out, Check{Level: Info, Title: i18n.T("ops.doctor.running")})
	default:
		out = append(out, Check{Level: Info, Title: i18n.T("ops.doctor.closed")})
	}
	return out
}

func (e *Env) accountChecks(st Status) []Check {
	var out []Check
	var owner, names []string
	for _, p := range st.Profiles {
		names = append(names, p.Name)
		if p.SignedIn {
			owner = append(owner, p.Name)
		}
	}
	switch {
	case st.Account == "":
		out = append(out, Check{Level: Info, Title: i18n.T("ops.doctor.signed-out")})
	case len(owner) > 0:
		out = append(out, Check{Level: OK, Title: i18n.T("ops.doctor.signed-in", "name", strings.Join(owner, ", "))})
	default:
		out = append(out, Check{Level: Warn, Title: i18n.T("ops.doctor.unsaved"),
			Detail: i18n.T("ops.list.account", "id", short(st.Account)), Fix: i18n.T("ops.doctor.unsaved.fix")})
	}
	if p, ok := e.Vault.Find(st.Current); ok && e.matches(p) != nil {
		out = append(out, Check{Level: Warn, Title: i18n.T("ops.doctor.mismatch", "name", p.Name),
			Detail: i18n.T("ops.doctor.mismatch.detail"), Fix: i18n.T("ops.doctor.mismatch.fix")})
	}
	return append(out, Check{Level: Info, Title: i18n.T("ops.doctor.profiles", "n", len(names)), Detail: strings.Join(names, ", ")})
}

func (e *Env) listChecks(st Status) []Check {
	var out []Check
	if len(st.Lists) == 0 {
		out = append(out, Check{Level: Info, Title: i18n.T("ops.doctor.no-lists"), Detail: i18n.T("ops.doctor.no-lists.detail")})
	}
	index, err := e.Index(e.Projects)
	if err != nil {
		return append(out, Check{Level: Warn, Title: i18n.T("ops.doctor.transcripts-unreadable"), Detail: err.Error()})
	}
	for _, l := range st.Lists {
		c := Check{Level: OK, Title: i18n.T("ops.doctor.list", "label", l.Label, "n", l.Chats)}
		if l.SignedIn {
			c.Detail = i18n.T("ops.doctor.list.signed-in")
		}
		if l.Linked {
			c = Check{Level: Fail, Title: c.Title, Detail: i18n.T("ops.doctor.linked"), Fix: i18n.T("ops.doctor.linked.fix")}
		}
		out = append(out, c)
		if n := ghosts(l, index); n > 0 {
			out = append(out, Check{Level: Info, Title: i18n.T("ops.doctor.ghosts", "n", n, "label", l.Label), Detail: i18n.T("ops.doctor.ghosts.detail")})
		}
	}
	switch orphans, _, err := e.Orphans(); {
	case err != nil:
		out = append(out, Check{Level: Warn, Title: i18n.T("ops.doctor.transcripts-unreadable"), Detail: err.Error()})
	case len(orphans) > 0:
		out = append(out, Check{Level: Warn, Title: i18n.T("ops.doctor.lost", "n", len(orphans)), Fix: i18n.T("ops.doctor.lost.fix")})
	default:
		out = append(out, Check{Level: OK, Title: i18n.T("ops.doctor.no-lost")})
	}
	return out
}

// ghosts counts a list's chats whose transcript is gone: they open empty.
func ghosts(l List, index map[string]string) int {
	chats, _ := chatsOf(l, index) // an unreadable list was already reported by Status
	n := 0
	for _, c := range chats {
		if !c.HasHistory {
			n++
		}
	}
	return n
}

// retentionChecks explain how long Claude Code keeps transcripts. Since Claude Code 2.1.248 it
// keeps chats started or continued in Desktop at any age unless desktopSessionCleanupPeriodDays
// is set; chats only ever used in a terminal go after cleanupPeriodDays (default 30).
func (e *Env) retentionChecks() []Check {
	path := filepath.Join(filepath.Dir(e.Projects), "settings.json")
	var s struct {
		Cleanup        *int `json:"cleanupPeriodDays"`
		DesktopCleanup *int `json:"desktopSessionCleanupPeriodDays"`
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s) // a broken file leaves Claude Code's defaults, as far as we can tell
	}
	desktop := Check{Level: OK, Title: i18n.T("ops.doctor.desktop-kept")}
	if s.DesktopCleanup != nil {
		desktop = Check{Level: Warn, Title: i18n.T("ops.doctor.desktop-cleanup", "n", *s.DesktopCleanup),
			Fix: i18n.T("ops.doctor.desktop-cleanup.fix", "path", path)}
	}
	days := 30
	if s.Cleanup != nil {
		days = *s.Cleanup
	}
	return []Check{desktop, {Level: Info, Title: i18n.T("ops.doctor.terminal-cleanup", "n", days),
		Fix: i18n.T("ops.doctor.terminal-cleanup.fix", "path", path)}}
}
