// Package ops is every action Claude Switcher takes: switching accounts, copying, moving,
// merging and rescuing chats, undo, and the checks behind doctor. It is the only package that
// writes Claude's data, and it only writes while Claude Desktop is closed. It never quits or
// launches Desktop; callers do, so a command can leave Desktop closed.
package ops

import (
	"cmp"
	"errors"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/claude"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// Options choose the folders to work on; "" finds the usual one.
type Options struct {
	DataDir     string // Desktop's data folder; set, it is used as is and nothing runs or launches
	VaultDir    string // our state, platform.VaultDir() by default
	ProjectsDir string // Claude Code transcripts, platform.ProjectsDir() by default
}

// Env is what every action works on. Its fields can be replaced in tests.
type Env struct {
	Install  platform.Install
	Desktop  platform.Desktop
	Vault    store.Vault
	Projects string
	Items    []string // login items swapped per profile
	Now      func() time.Time

	Index     func(projectsDir string) (map[string]string, error)
	ReadMeta  func(path string) (transcript.Meta, error)
	QuickMeta func(path string) (transcript.Meta, error)
}

// Open finds Claude Desktop (or uses DataDir) and the folders around it.
func Open(o Options) (*Env, error) {
	in := platform.Install{Kind: platform.Custom, DataDir: o.DataDir}
	if o.DataDir == "" {
		found, err := platform.Detect()
		if errors.Is(err, platform.ErrNotFound) || err == nil && len(found) == 0 {
			return nil, errf(NoInstall)
		}
		if err != nil {
			return nil, err
		}
		in = found[0]
	}
	return &Env{
		Install:   in,
		Desktop:   platform.NewDesktop(in),
		Vault:     store.Vault{Dir: cmp.Or(o.VaultDir, platform.VaultDir())},
		Projects:  cmp.Or(o.ProjectsDir, platform.ProjectsDir()),
		Items:     platform.LoginItems,
		Now:       time.Now,
		Index:     transcript.Index,
		ReadMeta:  transcript.ReadMeta,
		QuickMeta: transcript.QuickMeta,
	}, nil
}

// Code names why an action refused. The text for code c is the i18n key "ops.err.<c>".
type Code string

const (
	NoInstall      Code = "no-install"      // no Claude Desktop on this computer
	DesktopRunning Code = "desktop-running" // quit Desktop first (see WaitClosed)
	NotSignedIn    Code = "not-signed-in"   // Desktop has no login to save
	NoProfile      Code = "no-profile"      // name: no such profile
	NoCurrent      Code = "no-current"      // save the signed-in account before adding another
	ProfileExists  Code = "profile-exists"  // name
	BadName        Code = "bad-name"        // name
	OtherAccount   Code = "other-account"   // name: Desktop is signed into another account than this profile
	ProfileInUse   Code = "profile-in-use"  // name
	AlreadyCurrent Code = "already-current" // name
	NothingToUndo  Code = "nothing-to-undo"
	SameList       Code = "same-list"
	NoList         Code = "no-list"        // ref
	AmbiguousList  Code = "ambiguous-list" // ref
	ChatNotFound   Code = "chat-not-found" // ref
	AmbiguousChat  Code = "ambiguous-chat" // ref, n
	NoHistory      Code = "no-history"     // the chat's transcript is gone
	BadFormat      Code = "bad-format"     // format
)

// Error is a refusal the user can act on. Args are the name, value pairs its text needs.
type Error struct {
	Code Code
	Args []any
}

func (e *Error) Error() string { return i18n.T("ops.err."+string(e.Code), e.Args...) }

// Is reports whether err is a refusal with code c.
func Is(err error, c Code) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == c
}

func errf(c Code, args ...any) error { return &Error{Code: c, Args: args} }

// closed refuses to go on while Desktop runs: it would write over our changes.
func (e *Env) closed() error {
	running, err := e.Desktop.Running()
	if err != nil {
		return err
	}
	if running {
		return errf(DesktopRunning)
	}
	return nil
}

// live is the account Desktop is signed into.
func (e *Env) live() string { return claude.LiveAccount(e.Install.DataDir) }
