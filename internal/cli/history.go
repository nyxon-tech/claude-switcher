package cli

import (
	"slices"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

func (a *app) undo(cmd *cobra.Command, _ []string) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	js, err := env.History()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(js, func(j store.Journal) bool { return !j.Undone })
	if i < 0 {
		return &ops.Error{Code: ops.NothingToUndo}
	}
	if err := a.confirm(cmd.Context(), i18n.T("cli.undo.confirm", "summary", js[i].Summary)); err != nil {
		return err
	}
	var j store.Journal
	err = a.write(cmd.Context(), env, false, func() (err error) {
		j, err = env.Undo()
		return err
	})
	if err != nil {
		return err
	}
	return a.result(journalOf(j), "cli.undo.done", "summary", j.Summary)
}

type journalJSON struct {
	ID      string    `json:"id"`
	Action  string    `json:"action"`
	At      time.Time `json:"at"`
	Summary string    `json:"summary"`
	Files   int       `json:"files"`
	Undone  bool      `json:"undone"`
}

func journalOf(j store.Journal) journalJSON {
	return journalJSON{ID: j.ID, Action: j.Action, At: j.At, Summary: j.Summary, Files: len(j.Entries), Undone: j.Undone}
}

func (a *app) historyCommand() *cobra.Command {
	cmd := a.command("history", "history", "chats", nargs(0, 0), func(*cobra.Command, []string) error {
		env, err := a.open()
		if err != nil {
			return err
		}
		js, err := env.History()
		if err != nil {
			return err
		}
		if a.flags.json {
			out := make([]journalJSON, 0, len(js))
			for _, j := range js {
				out = append(out, journalOf(j))
			}
			return a.json(out)
		}
		if len(js) == 0 {
			a.note(a.out, i18n.T("cli.history.none"))
			return nil
		}
		now := time.Now()
		rows := make([][]string, len(js))
		for i, j := range js {
			rows[i] = []string{a.st.muted.Render(a.txt(i18n.Ago(j.At, now))), "", a.txt(i18n.N(int64(len(j.Entries))))}
		}
		headers := []string{i18n.T("cli.history.when"), i18n.T("cli.history.change"), i18n.T("cli.history.files")}
		a.fitColumn(headers, rows, 1, func(i, width int) string {
			if !js[i].Undone {
				return a.fit(js[i].Summary, width)
			}
			tag := a.st.dim.Render(a.txt(i18n.T("cli.history.undone")))
			return a.line(a.st.dim.Strikethrough(true).Render(a.fit(js[i].Summary, width-lipgloss.Width(tag)-1)), tag)
		})
		a.print(a.out, a.table(headers, rows, 2))
		return nil
	})
	cmd.Aliases = []string{"log"}
	return cmd
}
