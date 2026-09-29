package ops

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/claude"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

// Status is the whole picture: which account Desktop uses, the saved profiles and the chat lists.
type Status struct {
	Install  platform.Install
	Account  string // account Desktop is signed into, "" when signed out
	Current  string // profile in use, "" before the first save; it may not be saved yet (see Add)
	Running  bool
	Profiles []Profile
	Lists    []List
}

// Profile is a saved login.
type Profile struct {
	Name     string
	Account  string
	Saved    time.Time
	Current  bool // the profile in use
	SignedIn bool // Desktop is signed into its account right now
	Chats    int  // chats in its account's lists
}

// List is one chat list (an account and organization) as the user sees it.
type List struct {
	claude.Space
	Label    string   // the profiles of its account, or "account 1a2b3c4d"
	Profiles []string // profiles saved for its account
	SignedIn bool
	Linked   bool // a junction or symlink: Desktop never writes through it
	Chats    int
}

// Status reads the current state. It changes nothing.
func (e *Env) Status() (Status, error) {
	running, err := e.Desktop.Running()
	if err != nil {
		return Status{}, err
	}
	profiles, err := e.Vault.Profiles()
	if err != nil {
		return Status{}, err
	}
	lists, err := e.lists(profiles)
	if err != nil {
		return Status{}, err
	}
	s := Status{Install: e.Install, Account: e.live(), Current: e.Vault.Current(), Running: running, Lists: lists}
	for _, p := range profiles {
		chats := 0
		for _, l := range lists {
			if l.Account == p.Account {
				chats += l.Chats
			}
		}
		s.Profiles = append(s.Profiles, Profile{
			Name: p.Name, Account: p.Account, Saved: p.Saved, Chats: chats,
			Current:  p.Name == s.Current,
			SignedIn: p.Account != "" && p.Account == s.Account,
		})
	}
	return s, nil
}

// Lists returns every chat list.
func (e *Env) Lists() ([]List, error) {
	profiles, err := e.Vault.Profiles()
	if err != nil {
		return nil, err
	}
	return e.lists(profiles)
}

func (e *Env) lists(profiles []store.Profile) ([]List, error) {
	spaces, err := claude.Spaces(e.Install.DataDir)
	if err != nil {
		return nil, err
	}
	live := e.live()
	orgs := map[string]int{}
	for _, s := range spaces {
		orgs[s.Account]++
	}
	out := make([]List, 0, len(spaces))
	for _, s := range spaces {
		cards, err := s.Cards()
		if err != nil {
			return nil, err
		}
		l := List{Space: s, SignedIn: s.Account == live, Linked: s.Linked(), Chats: len(cards)}
		for _, p := range profiles {
			if p.Account == s.Account {
				l.Profiles = append(l.Profiles, p.Name)
			}
		}
		l.Label = strings.Join(l.Profiles, ", ")
		if l.Label == "" {
			l.Label = i18n.T("ops.list.account", "id", short(s.Account))
		}
		if orgs[s.Account] > 1 {
			l.Label = i18n.T("ops.list.org", "label", l.Label, "id", short(s.Org))
		}
		out = append(out, l)
	}
	return out, nil
}

// FindList finds the one list ref names: a profile name, "signed-in" or an account id prefix,
// optionally followed by "/" and an org id prefix.
func (e *Env) FindList(ref string) (List, error) {
	if ref == "" {
		return List{}, errf(NoList, "ref", ref)
	}
	lists, err := e.Lists()
	if err != nil {
		return List{}, err
	}
	name, org, _ := strings.Cut(ref, "/")
	account := func(l List) bool { return hasPrefixFold(l.Account, name) }
	if p, ok := e.Vault.Find(name); ok && p.Account != "" {
		account = func(l List) bool { return l.Account == p.Account }
	}
	if name == "signed-in" {
		account = func(l List) bool { return l.SignedIn }
	}
	match := func(l List) bool { return account(l) && hasPrefixFold(l.Org, org) }
	var hits []List
	for _, l := range lists {
		if match(l) {
			hits = append(hits, l)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return List{}, errf(NoList, "ref", ref)
	}
	return List{}, errf(AmbiguousList, "ref", ref)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// short is the first 8 characters of an id, enough to tell accounts apart.
func short(id string) string { return id[:min(8, len(id))] }

// sameDir reports whether two folders are one, links included.
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(sa, sb)
}
