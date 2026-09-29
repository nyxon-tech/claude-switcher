package ops

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/claude"
)

// Chat is one card in a chat list.
type Chat struct {
	ID         string // card id, "local_<uuid>"
	Title      string // on one line; "" when the chat has none yet
	Project    string // folder name of Cwd
	Cwd        string
	Session    string   // current transcript id
	Prior      []string // older transcript ids of the same chat
	Model      string
	Created    time.Time
	Last       time.Time // last activity, else creation; zero when unknown
	Archived   bool
	HasHistory bool   // the transcript is on disk (a chat with no message yet counts as having it)
	Transcript string // path of the current transcript, "" when there is none
	List       List
	Path       string // the card file
}

// Chats lists the chats of one list, newest first.
func (e *Env) Chats(l List) ([]Chat, error) {
	index, err := e.Index(e.Projects)
	if err != nil {
		return nil, err
	}
	return chatsOf(l, index)
}

func chatsOf(l List, index map[string]string) ([]Chat, error) {
	cards, err := l.Cards()
	if err != nil {
		return nil, err
	}
	out := make([]Chat, 0, len(cards))
	for _, c := range cards {
		path, found := index[c.Session()]
		out = append(out, Chat{
			ID: c.ID(), Title: strings.Join(strings.Fields(c.Title()), " "),
			Project: project(c.Cwd()), Cwd: c.Cwd(),
			Session: c.Session(), Prior: c.PriorSessions(), Model: c.Model(),
			Created: msTime(c.Created()), Last: lastOf(c), Archived: c.Archived(),
			HasHistory: c.Session() == "" || found, Transcript: path,
			List: l, Path: c.Path,
		})
	}
	slices.SortStableFunc(out, func(a, b Chat) int { return b.Last.Compare(a.Last) })
	return out, nil
}

// lastOf is when a chat was last used: two lists can hold the same chat at different points.
func lastOf(c *claude.Card) time.Time {
	if ms := c.LastActivity(); ms > 0 {
		return msTime(ms)
	}
	return msTime(c.Created())
}

func msTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

func project(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Base(cwd)
}

// pickChats resolves chat references (ids, id prefixes with or without "local_", or transcript
// id prefixes) to one chat each.
func pickChats(chats []Chat, refs []string) ([]Chat, error) {
	var out []Chat
	seen := map[string]bool{}
	for _, ref := range refs {
		hits := matchChats(chats, ref)
		switch {
		case len(hits) == 0:
			return nil, errf(ChatNotFound, "ref", ref)
		case len(hits) > 1:
			return nil, errf(AmbiguousChat, "ref", ref, "n", len(hits))
		case !seen[hits[0].ID]:
			seen[hits[0].ID] = true
			out = append(out, hits[0])
		}
	}
	return out, nil
}

func matchChats(chats []Chat, ref string) []Chat {
	if ref == "" {
		return nil
	}
	var hits []Chat
	for _, c := range chats {
		if c.ID == ref || c.Session == ref {
			return []Chat{c}
		}
		if hasPrefixFold(c.ID, ref) || hasPrefixFold(c.ID, "local_"+ref) || c.Session != "" && hasPrefixFold(c.Session, ref) {
			hits = append(hits, c)
		}
	}
	return hits
}
