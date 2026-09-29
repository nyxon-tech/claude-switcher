package cli

import (
	"context"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

func (a *app) chatCommands() []*cobra.Command {
	var lost bool
	var limit int
	chats := a.command("chats [list]", "chats", "chats", nargs(0, 1), func(_ *cobra.Command, args []string) error {
		return a.chats(args, lost, limit)
	})
	chats.Flags().BoolVar(&lost, "lost", false, a.txt(i18n.T("cli.flag.lost")))
	chats.Flags().IntVar(&limit, "limit", 0, a.txt(i18n.T("cli.flag.limit")))

	var to string
	merge := a.forceQuitFlag(a.command("merge --to <list>", "merge", "chats", nargs(0, 0), func(cmd *cobra.Command, _ []string) error {
		if to == "" {
			return usageError(errText("cli.err.merge"), cmd)
		}
		env, err := a.open()
		if err != nil {
			return err
		}
		dst, err := env.FindList(to)
		if err != nil {
			return err
		}
		plan, err := env.PlanMerge(dst)
		if err != nil {
			return err
		}
		return a.apply(cmd.Context(), env, plan)
	}))
	merge.Flags().StringVar(&to, "to", "", a.txt(i18n.T("cli.flag.to")))

	return []*cobra.Command{chats, a.transfer(false), a.transfer(true), merge, a.rescueCommand(), a.exportCommand(),
		a.forceQuitFlag(a.command("undo", "undo", "chats", nargs(0, 0), a.undo)), a.historyCommand()}
}

type chatJSON struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Project    string    `json:"project"`
	Cwd        string    `json:"cwd"`
	Session    string    `json:"session"`
	Prior      []string  `json:"prior"`
	Model      string    `json:"model"`
	Created    time.Time `json:"created,omitzero"`
	LastUsed   time.Time `json:"lastUsed,omitzero"`
	Archived   bool      `json:"archived"`
	HasHistory bool      `json:"hasHistory"`
	List       string    `json:"list"`
	Account    string    `json:"account"`
	Org        string    `json:"org"`
}

func (a *app) chats(args []string, lost bool, limit int) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	if lost {
		return a.lostChats(env, limit)
	}
	lists, err := env.Lists()
	if len(args) == 1 {
		var l ops.List
		l, err = env.FindList(args[0])
		lists = []ops.List{l}
	}
	if err != nil {
		return err
	}
	chats, err := allChats(env, lists)
	if err != nil {
		return err
	}
	total := len(chats)
	if limit > 0 && total > limit {
		chats = chats[:limit]
	}
	if a.flags.json {
		out := make([]chatJSON, 0, len(chats))
		for _, c := range chats {
			out = append(out, chatJSON{ID: c.ID, Title: c.Title, Project: c.Project, Cwd: c.Cwd, Session: c.Session,
				Prior: append([]string{}, c.Prior...), Model: c.Model, Created: c.Created, LastUsed: c.Last, Archived: c.Archived,
				HasHistory: c.HasHistory, List: c.List.Label, Account: c.List.Account, Org: c.List.Org})
		}
		return a.json(out)
	}
	if total == 0 {
		a.note(a.out, i18n.T("cli.chats.none"))
		return nil
	}
	a.printChats(chats, len(lists) > 1)
	if len(chats) < total {
		a.note(a.out, i18n.T("cli.chats.shown", "shown", len(chats), "n", total))
	}
	return nil
}

func (a *app) printChats(chats []ops.Chat, withList bool) {
	now := time.Now()
	project := a.width >= 72 // a narrow window has no room for the project
	headers := []string{i18n.T("cli.chats.id"), i18n.T("cli.chats.title")}
	if project {
		headers = append(headers, i18n.T("cli.chats.project"))
	}
	headers = append(headers, i18n.T("cli.chats.last"))
	if withList {
		headers = append(headers, i18n.T("cli.chats.list"))
	}
	rows := make([][]string, len(chats))
	for i, c := range chats {
		row := []string{a.st.muted.Render(shortID(c.ID)), ""}
		if project {
			row = append(row, a.st.muted.Render(a.fit(c.Project, a.labelWidth())))
		}
		row = append(row, a.st.muted.Render(a.txt(ago(c.Last, now))))
		if withList {
			row = append(row, a.st.muted.Render(a.fit(c.List.Label, a.labelWidth())))
		}
		rows[i] = row
	}
	a.fitColumn(headers, rows, 1, func(i, width int) string { return a.chatTitle(chats[i], width) })
	a.print(a.out, a.table(headers, rows))
}

// labelWidth is the room for a project folder or a list label in a table of chats.
func (a *app) labelWidth() int { return min(20, max(8, (a.width-40)/3)) }

// chatTitle is a chat's title cut to width, marked when its history is gone and dimmed when it
// is archived.
func (a *app) chatTitle(c ops.Chat, width int) string {
	var tag string
	if !c.HasHistory {
		tag = a.st.warn.Render(a.txt(i18n.T("cli.chats.no-history")))
		width -= lipgloss.Width(tag) + 3 // "! " before and a space after the title
	}
	title := a.fit(c.Title, max(width, 4))
	if c.Title == "" {
		title = a.st.dim.Render(a.txt(i18n.T("cli.chats.untitled")))
	}
	switch {
	case tag != "":
		return a.line(a.st.warn.Render("!"), title, tag)
	case c.Archived:
		return a.st.dim.Render(title)
	}
	return title
}

// shortID is the part of a chat id people type: 8 characters after "local_".
func shortID(id string) string {
	id = strings.TrimPrefix(id, "local_")
	return id[:min(8, len(id))]
}

func ago(t, now time.Time) string {
	if t.IsZero() {
		return i18n.T("time.never")
	}
	return i18n.Ago(t, now)
}

// allChats reads the chats of every list given, newest first.
func allChats(env *ops.Env, lists []ops.List) ([]ops.Chat, error) {
	var out []ops.Chat
	for _, l := range lists {
		chats, err := env.Chats(l)
		if err != nil {
			return nil, err
		}
		out = append(out, chats...)
	}
	slices.SortStableFunc(out, func(x, y ops.Chat) int { return y.Last.Compare(x.Last) })
	return out, nil
}

// transfer is copy, or move when move is set.
func (a *app) transfer(move bool) *cobra.Command {
	name := map[bool]string{false: "copy", true: "move"}[move]
	var from, to string
	var ids []string
	cmd := a.forceQuitFlag(a.command(name+" <chat ids...>", name, "chats", nargs(0, -1),
		func(cmd *cobra.Command, args []string) error {
			refs := append(ids, args...)
			if from == "" || to == "" || len(refs) == 0 {
				return usageError(errText("cli.err.transfer"), cmd)
			}
			env, err := a.open()
			if err != nil {
				return err
			}
			src, err := env.FindList(from)
			if err != nil {
				return err
			}
			dst, err := env.FindList(to)
			if err != nil {
				return err
			}
			plan, err := env.PlanTransfer(src, dst, refs, move)
			if err != nil {
				return err
			}
			return a.apply(cmd.Context(), env, plan)
		}))
	cmd.Flags().StringVar(&from, "from", "", a.txt(i18n.T("cli.flag.from")))
	cmd.Flags().StringVar(&to, "to", "", a.txt(i18n.T("cli.flag.to")))
	cmd.Flags().StringSliceVar(&ids, "chat", nil, a.txt(i18n.T("cli.flag.chat")))
	return cmd
}

// pointer is the › that leads a plan or a fix.
func (a *app) pointer() string { return a.st.accent.Render("›") }

// apply shows a plan, asks, and carries it out once Desktop is closed.
func (a *app) apply(ctx context.Context, env *ops.Env, p ops.Plan) error {
	a.say(a.err, a.pointer(), a.st.plain, i18n.T("cli.plan."+p.Kind, "n", len(p.Steps), "from", p.From.Label, "to", p.To.Label))
	a.note(a.err, i18n.T("cli.plan.counts", "create", p.Count(ops.Create), "update", p.Count(ops.Update), "skip", p.Count(ops.SkipNewer)))
	// A move still removes the source cards when the target already has newer copies.
	if p.Count(ops.Create)+p.Count(ops.Update) == 0 && (p.Kind != ops.Move || len(p.Steps) == 0) {
		return a.changed(p.Kind, p.From.Label, p.To.Label, ops.Result{Summary: i18n.T("cli.plan.nothing", "to", p.To.Label), Skipped: len(p.Steps)}, false)
	}
	if err := a.confirm(ctx, i18n.T("cli.confirm.proceed")); err != nil {
		return err
	}
	var res ops.Result
	err := a.write(ctx, env, false, func() (err error) {
		res, err = env.Apply(p)
		return err
	})
	if err != nil {
		return err
	}
	return a.changed(p.Kind, p.From.Label, p.To.Label, res, true)
}

type changeJSON struct {
	Kind    string `json:"kind"`
	From    string `json:"from,omitempty"`
	To      string `json:"to"`
	Summary string `json:"summary"`
	Created int    `json:"created"`
	Updated int    `json:"updated"`
	Skipped int    `json:"skipped"`
	Removed int    `json:"removed"`
}

// changed reports a finished change; wrote says whether it wrote anything undo can take back.
func (a *app) changed(kind, from, to string, res ops.Result, wrote bool) error {
	if a.flags.json {
		return a.json(changeJSON{Kind: kind, From: from, To: to, Summary: res.Summary,
			Created: res.Created, Updated: res.Updated, Skipped: res.Skipped, Removed: res.Removed})
	}
	a.done(a.out, res.Summary)
	if wrote {
		a.note(a.out, i18n.T("cli.undo.hint"))
	}
	return nil
}
