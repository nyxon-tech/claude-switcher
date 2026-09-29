package ops

import (
	"cmp"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nyxon-tech/claude-switcher/v3/internal/claude"
	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

// Kinds of change, also the action names in the journal (v2 used the same).
const (
	Copy   = "copy"
	Move   = "move"
	Merge  = "merge"
	Rescue = "rescue"
)

// Action is what one step does to the target list. Newest wins: a card is only written over
// when the copy being written was used more recently.
type Action string

const (
	Create    Action = "create"     // the target lacks the chat
	Update    Action = "update"     // the target has an older copy; it is backed up and replaced
	SkipNewer Action = "skip-newer" // the target's copy is as new or newer; it is left alone
)

// Step is one chat of a plan.
type Step struct {
	Action Action
	Chat   Chat   // the copy to write
	Target string // its card file in the target list
}

// Plan is a change worked out before anything is written, for the caller to show and confirm.
type Plan struct {
	Kind  string // Copy, Move or Merge
	From  List   // zero for Merge, which reads every other list
	To    List
	Steps []Step
}

// Count is how many steps take action a.
func (p Plan) Count(a Action) int {
	n := 0
	for _, s := range p.Steps {
		if s.Action == a {
			n++
		}
	}
	return n
}

// Result is what a change did.
type Result struct {
	Summary string
	Created int
	Updated int
	Skipped int // the target already had the chat as new or newer
	Removed int // source cards removed by a move
}

// PlanTransfer plans copying (or moving) chats from one list to another. ids are chat
// references as Chats reports them; prefixes work. A move removes each source card once the
// target holds the chat, whether it was written or already newer there.
func (e *Env) PlanTransfer(from, to List, ids []string, move bool) (Plan, error) {
	if from.Dir == "" || to.Dir == "" {
		return Plan{}, errf(NoList, "ref", "")
	}
	if sameDir(from.Dir, to.Dir) {
		return Plan{}, errf(SameList)
	}
	chats, err := e.Chats(from)
	if err != nil {
		return Plan{}, err
	}
	picked, err := pickChats(chats, ids)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Kind: Copy, From: from, To: to}
	if move {
		p.Kind = Move
	}
	for _, c := range picked {
		target := filepath.Join(to.Dir, c.ID+".json")
		p.Steps = append(p.Steps, Step{Action: decide(c.Last, target), Chat: c, Target: target})
	}
	return p, nil
}

// PlanMerge plans copying into a list every chat it is missing or holds an older copy of,
// taking the newest copy found in any other list.
func (e *Env) PlanMerge(to List) (Plan, error) {
	if to.Dir == "" {
		return Plan{}, errf(NoList, "ref", "")
	}
	index, err := e.Index(e.Projects)
	if err != nil {
		return Plan{}, err
	}
	lists, err := e.Lists()
	if err != nil {
		return Plan{}, err
	}
	have := map[string]time.Time{}
	targetChats, err := chatsOf(to, index)
	if err != nil {
		return Plan{}, err
	}
	for _, c := range targetChats {
		have[c.ID] = c.Last
	}
	newest := map[string]Chat{}
	for _, l := range lists {
		if sameDir(l.Dir, to.Dir) {
			continue
		}
		chats, err := chatsOf(l, index)
		if err != nil {
			return Plan{}, err
		}
		for _, c := range chats {
			if last, ok := have[c.ID]; ok && !c.Last.After(last) {
				continue
			}
			if n, ok := newest[c.ID]; !ok || c.Last.After(n.Last) {
				newest[c.ID] = c
			}
		}
	}
	p := Plan{Kind: Merge, To: to}
	for _, c := range slices.SortedFunc(maps.Values(newest), newestFirst) {
		action := Create
		if _, ok := have[c.ID]; ok {
			action = Update
		}
		p.Steps = append(p.Steps, Step{Action: action, Chat: c, Target: filepath.Join(to.Dir, c.ID+".json")})
	}
	return p, nil
}

func newestFirst(a, b Chat) int { return cmp.Or(b.Last.Compare(a.Last), strings.Compare(a.ID, b.ID)) }

// decide applies newest wins to a copy used at last and the target card file.
func decide(last time.Time, target string) Action {
	if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
		return Create
	}
	var have time.Time // an unreadable target counts as oldest, so it is backed up and replaced
	if c, err := claude.ReadCard(target); err == nil {
		have = lastOf(c)
	}
	if last.After(have) {
		return Update
	}
	return SkipNewer
}

// Apply carries out a plan, journaled so Undo restores every byte. Each step is decided again
// against the files as they are now, so a stale plan never writes over a newer card.
func (e *Env) Apply(p Plan) (Result, error) {
	var res Result
	if err := e.closed(); err != nil {
		return res, err
	}
	j, err := e.Vault.NewJournal(p.Kind, e.Now())
	if err != nil {
		return res, err
	}
	for _, s := range p.Steps {
		if err = apply(j, p.Kind == Move, s, &res); err != nil {
			break
		}
	}
	res.Summary = summary(p)
	if serr := j.Save(res.Summary); err == nil {
		err = serr
	}
	return res, err
}

func apply(j *store.Journal, move bool, s Step, res *Result) error {
	data, err := os.ReadFile(s.Chat.Path)
	if err != nil {
		return err
	}
	card, err := claude.ParseCard(data)
	if err != nil {
		return err
	}
	if err := writeCard(j, s.Target, data, decide(lastOf(card), s.Target), res); err != nil {
		return err
	}
	if !move {
		return nil
	}
	if err := j.Removed(s.Chat.Path); err != nil {
		return err
	}
	if err := os.Remove(s.Chat.Path); err != nil {
		return err
	}
	res.Removed++
	return nil
}

// writeCard puts a card's bytes at target as action a says, journaling first. The bytes are copied
// as they are, so every field survives.
func writeCard(j *store.Journal, target string, data []byte, a Action, res *Result) error {
	var err error
	switch a {
	case Create:
		err = j.Created(target)
	case Update:
		err = j.Removed(target)
	default:
		res.Skipped++
		return nil
	}
	if err == nil {
		err = claude.WriteFileAtomic(target, data)
	}
	if err != nil {
		return err
	}
	if a == Create {
		res.Created++
	} else {
		res.Updated++
	}
	return nil
}

func summary(p Plan) string {
	n := len(p.Steps)
	switch p.Kind {
	case Merge:
		return i18n.T("ops.done.merge", "n", n, "to", p.To.Label)
	case Move:
		return i18n.T("ops.done.move", "n", n, "from", p.From.Label, "to", p.To.Label)
	}
	return i18n.T("ops.done.copy", "n", n, "from", p.From.Label, "to", p.To.Label)
}
