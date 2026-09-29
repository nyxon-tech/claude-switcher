package cli

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

func (a *app) rescueCommand() *cobra.Command {
	var to string
	var all bool
	cmd := a.forceQuitFlag(a.command("rescue [chat ids...]", "rescue", "chats", nargs(0, -1),
		func(cmd *cobra.Command, args []string) error {
			env, err := a.open()
			if err != nil {
				return err
			}
			if !all && len(args) == 0 {
				return a.lostChats(env, 0)
			}
			return a.rescue(cmd.Context(), env, cmp.Or(to, "signed-in"), args, all)
		}))
	cmd.Flags().StringVar(&to, "to", "", a.txt(i18n.T("cli.flag.rescue-to")))
	cmd.Flags().BoolVar(&all, "all", false, a.txt(i18n.T("cli.flag.all")))
	return cmd
}

func (a *app) rescue(ctx context.Context, env *ops.Env, to string, refs []string, all bool) error {
	dst, err := env.FindList(to)
	if err != nil {
		return err
	}
	orphans, _, err := env.Orphans()
	if err != nil {
		return err
	}
	if all {
		refs = nil
		for _, o := range orphans {
			refs = append(refs, o.Meta.Session)
		}
	}
	for _, ref := range refs {
		if !slices.ContainsFunc(orphans, func(o ops.Orphan) bool { return ref != "" && prefixFold(o.Meta.Session, ref) }) {
			return &ops.Error{Code: ops.ChatNotFound, Args: []any{"ref", ref}}
		}
	}
	if len(refs) == 0 {
		return a.changed(ops.Rescue, "", dst.Label, ops.Result{Summary: i18n.T("ops.doctor.no-lost")}, false)
	}
	a.say(a.err, a.pointer(), a.st.plain, i18n.T("cli.plan.rescue", "n", len(refs), "to", dst.Label))
	if err := a.confirm(ctx, i18n.T("cli.confirm.proceed")); err != nil {
		return err
	}
	var res ops.Result
	err = a.write(ctx, env, false, func() (err error) {
		res, err = env.Rescue(dst, refs)
		return err
	})
	if err != nil {
		return err
	}
	return a.changed(ops.Rescue, "", dst.Label, res, true)
}

type lostJSON struct {
	Session string    `json:"session"`
	Title   string    `json:"title"`
	Project string    `json:"project"`
	Cwd     string    `json:"cwd"`
	Model   string    `json:"model"`
	Start   time.Time `json:"start,omitzero"`
	End     time.Time `json:"end,omitzero"`
}

// lostChats shows the chats rescue can recover.
func (a *app) lostChats(env *ops.Env, limit int) error {
	orphans, left, err := env.Orphans()
	if err != nil {
		return err
	}
	total := len(orphans)
	if limit > 0 && total > limit {
		orphans = orphans[:limit]
	}
	if a.flags.json {
		out := make([]lostJSON, 0, len(orphans))
		for _, o := range orphans {
			out = append(out, lostJSON{Session: o.Meta.Session, Title: o.Title, Project: o.Project, Cwd: o.Meta.Cwd,
				Model: o.Meta.Model, Start: o.Meta.Start, End: o.Meta.End})
		}
		return a.json(map[string]any{"lost": out, "leftOut": left})
	}
	if total == 0 {
		a.done(a.out, i18n.T("ops.doctor.no-lost"))
		return nil
	}
	now := time.Now()
	rows := make([][]string, len(orphans))
	for i, o := range orphans {
		rows[i] = []string{a.st.muted.Render(shortID(o.Meta.Session)), "",
			a.st.muted.Render(a.fit(o.Project, a.labelWidth())), a.st.muted.Render(a.txt(ago(o.Meta.End, now)))}
	}
	headers := []string{i18n.T("cli.chats.id"), i18n.T("cli.chats.title"), i18n.T("cli.chats.project"), i18n.T("cli.chats.last")}
	a.fitColumn(headers, rows, 1, func(i, width int) string { return a.fit(orphans[i].Title, width) })
	a.warn(a.out, i18n.T("ops.doctor.lost", "n", total))
	a.print(a.out, a.table(headers, rows))
	if len(orphans) < total {
		a.note(a.out, i18n.T("cli.chats.shown", "shown", len(orphans), "n", total))
	}
	if left > 0 {
		a.note(a.out, i18n.T("cli.lost.left-out", "n", left))
	}
	a.note(a.out, i18n.T("cli.lost.hint"))
	return nil
}
