package ops

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nyxon-tech/claude-switcher/v3/internal/claude"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// Orphan is a chat whose history is on disk but that no chat list shows.
type Orphan struct {
	Meta    transcript.Meta
	Title   string // the title its card will get
	Project string
}

// Orphans finds lost chats, newest first, and how many other transcripts it left out as older
// parts or copies of chats that are offered or listed already.
//
// A chat spans several transcripts (a /clear, a restart or a resume starts a new one) and its
// card names them all: cliSessionId and priorCliSessionIds. So a transcript is lost when no card
// in any list names it, it was not deleted in the app, it holds a conversation, and no card
// carries its title in its folder. Transcripts of one lost chat share that title and folder and
// are offered once, at the newest.
func (e *Env) Orphans() ([]Orphan, int, error) {
	index, err := e.Index(e.Projects)
	if err != nil {
		return nil, 0, err
	}
	spaces, err := claude.Spaces(e.Install.DataDir)
	if err != nil {
		return nil, 0, err
	}
	known, titled := map[string]bool{}, map[string]bool{}
	for _, s := range spaces {
		cards, err := s.Cards()
		if err != nil {
			return nil, 0, err
		}
		for _, c := range cards {
			known[c.Session()] = true
			for _, id := range c.PriorSessions() {
				known[id] = true
			}
			if c.Title() != "" {
				titled[chatKey(c.Title(), c.Cwd())] = true
			}
		}
		deleted, err := s.Tombstones()
		if err != nil {
			return nil, 0, err
		}
		for _, id := range deleted {
			known[id] = true
		}
	}

	newest := map[string]Orphan{}
	candidates := 0
	for session, path := range index {
		if known[session] {
			continue
		}
		m, err := e.QuickMeta(path)
		if err != nil || !conversation(m) {
			continue
		}
		m.Session = session
		candidates++
		o := Orphan{Meta: m, Title: claude.TitleFor(m), Project: project(m.Cwd)}
		key := chatKey(o.Title, m.Cwd)
		if titled[key] {
			continue
		}
		if cur, ok := newest[key]; !ok || newerOrphan(o, cur) < 0 {
			newest[key] = o
		}
	}
	out := slices.SortedFunc(maps.Values(newest), newerOrphan)
	return out, candidates - len(out), nil
}

func newerOrphan(a, b Orphan) int {
	return cmp.Or(b.Meta.End.Compare(a.Meta.End), strings.Compare(a.Meta.Session, b.Meta.Session))
}

// conversation reports whether a transcript holds more than metadata. QuickMeta leaves the
// prompt and reply counts at 0, so a first prompt or a model is the sign.
func conversation(m transcript.Meta) bool {
	return m.Cwd != "" && (m.HasMessages() || m.FirstPrompt != "" || m.Model != "")
}

// chatKey identifies a chat by title and folder: Desktop titles every transcript of a chat alike.
func chatKey(title, cwd string) string {
	return strings.ToLower(claude.CleanTitle(title) + "|" + cwd)
}

// Rescue gives lost chats a card in list to, so Desktop shows them again. sessions are
// transcript ids (or prefixes) of chats Orphans offers.
func (e *Env) Rescue(to List, sessions []string) (Result, error) {
	if to.Dir == "" {
		return Result{}, errf(NoList, "ref", "")
	}
	orphans, _, err := e.Orphans()
	if err != nil {
		return Result{}, err
	}
	picked, err := pickOrphans(orphans, sessions)
	if err != nil {
		return Result{}, err
	}
	if err := e.closed(); err != nil {
		return Result{}, err
	}
	j, err := e.Vault.NewJournal(Rescue, e.Now())
	if err != nil {
		return Result{}, err
	}
	res := Result{Summary: i18n.T("ops.done.rescue", "n", len(picked), "to", to.Label)}
	for _, o := range picked {
		card := claude.NewCard(o.Meta)
		if err = writeCard(j, filepath.Join(to.Dir, card.ID()+".json"), card.Marshal(), Create, &res); err != nil {
			break
		}
	}
	if serr := j.Save(res.Summary); err == nil {
		err = serr
	}
	return res, err
}

func pickOrphans(orphans []Orphan, refs []string) ([]Orphan, error) {
	var out []Orphan
	seen := map[string]bool{}
	for _, ref := range refs {
		var hits []Orphan
		for _, o := range orphans {
			if ref != "" && hasPrefixFold(o.Meta.Session, ref) {
				hits = append(hits, o)
			}
		}
		switch {
		case len(hits) == 0:
			return nil, errf(ChatNotFound, "ref", ref)
		case len(hits) > 1:
			return nil, errf(AmbiguousChat, "ref", ref, "n", len(hits))
		case !seen[hits[0].Meta.Session]:
			seen[hits[0].Meta.Session] = true
			out = append(out, hits[0])
		}
	}
	return out, nil
}
