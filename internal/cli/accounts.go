package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

func (a *app) accountCommands() []*cobra.Command {
	list := a.command("list", "list", "accounts", nargs(0, 0), a.list)
	list.Aliases = []string{"ls", "profiles", "accounts"}

	var replace bool
	save := a.forceQuitFlag(a.command("save <name>", "save", "accounts", nargs(1, 1), func(cmd *cobra.Command, args []string) error {
		return a.save(cmd, args[0], replace)
	}))
	save.Flags().BoolVar(&replace, "replace", false, a.txt(i18n.T("cli.flag.replace")))

	add := a.forceQuitFlag(a.command("add <name>", "add", "accounts", nargs(1, 1), a.add))
	add.Aliases = []string{"new"}
	remove := a.command("remove <name>", "remove", "accounts", nargs(1, 1), a.remove)
	remove.Aliases = []string{"rm"}
	return []*cobra.Command{
		list,
		a.forceQuitFlag(a.command("switch <profile>", "switch", "accounts", nargs(1, 1), a.switchTo)),
		save, add,
		a.command("rename <old> <new>", "rename", "accounts", nargs(2, 2), a.rename),
		remove,
	}
}

type profileJSON struct {
	Name     string    `json:"name"`
	Account  string    `json:"account"`
	Saved    time.Time `json:"saved,omitzero"`
	Current  bool      `json:"current"`
	SignedIn bool      `json:"signedIn"`
	NotSaved bool      `json:"notSavedYet"` // in use, saved at the next switch (see add)
	Chats    int       `json:"chats"`
}

type listJSON struct {
	Label    string   `json:"label"`
	Account  string   `json:"account"`
	Org      string   `json:"org"`
	Profiles []string `json:"profiles"`
	SignedIn bool     `json:"signedIn"`
	Linked   bool     `json:"linked"`
	Chats    int      `json:"chats"`
	Dir      string   `json:"dir"`
}

func listOf(l ops.List) listJSON {
	return listJSON{Label: l.Label, Account: l.Account, Org: l.Org, Profiles: append([]string{}, l.Profiles...),
		SignedIn: l.SignedIn, Linked: l.Linked, Chats: l.Chats, Dir: l.Dir}
}

func (a *app) list(cmd *cobra.Command, _ []string) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	st, err := env.Status()
	if err != nil {
		return err
	}
	profiles := make([]profileJSON, 0, len(st.Profiles)+1)
	for _, p := range st.Profiles {
		profiles = append(profiles, profileJSON{Name: p.Name, Account: p.Account, Saved: p.Saved,
			Current: p.Current, SignedIn: p.SignedIn, Chats: p.Chats})
	}
	if _, saved := env.Vault.Find(st.Current); st.Current != "" && !saved {
		profiles = append(profiles, profileJSON{Name: st.Current, Current: true, NotSaved: true})
	}
	if a.flags.json {
		lists := make([]listJSON, 0, len(st.Lists))
		for _, l := range st.Lists {
			lists = append(lists, listOf(l))
		}
		return a.json(map[string]any{"account": st.Account, "current": st.Current, "desktopRunning": st.Running,
			"dataDir": st.Install.DataDir, "profiles": profiles, "lists": lists})
	}
	a.printProfiles(profiles)
	a.printUnowned(st.Lists)
	state := "ops.doctor.closed"
	if st.Running {
		state = "ops.doctor.running"
	}
	fmt.Fprintln(a.out)
	a.note(a.out, i18n.T(state))
	return nil
}

func (a *app) printProfiles(profiles []profileJSON) {
	if len(profiles) == 0 {
		a.note(a.out, i18n.T("cli.list.none"))
		return
	}
	now := time.Now()
	rows := make([][]string, len(profiles))
	for i, p := range profiles {
		var status []string
		if p.Current {
			status = append(status, i18n.T("cli.list.in-use"))
		}
		if p.SignedIn {
			status = append(status, i18n.T("cli.list.signed-in"))
		}
		saved := i18n.T("cli.list.not-saved")
		if !p.Saved.IsZero() {
			saved = i18n.Ago(p.Saved, now)
		}
		rows[i] = []string{"", a.txt(strings.Join(status, " · ")), a.txt(i18n.N(int64(p.Chats))), a.txt(saved)}
	}
	headers := []string{i18n.T("cli.list.profile"), i18n.T("cli.list.status"), i18n.T("cli.list.chats"), i18n.T("cli.list.saved")}
	a.fitColumn(headers, rows, 0, func(i, width int) string {
		p := profiles[i]
		mark, name := a.st.muted.Render("○"), a.fit(p.Name, width-2)
		switch {
		case p.SignedIn:
			mark = a.st.ok.Render("●")
		case p.Current:
			mark = a.st.warn.Render("●")
		}
		if p.Current {
			name = a.st.bold.Render(name)
		}
		return a.line(mark, name)
	})
	a.print(a.out, a.table(headers, rows, 2))
}

// printUnowned shows the chat lists no saved profile owns. Their label names the account.
func (a *app) printUnowned(lists []ops.List) {
	var rows [][]string
	for _, l := range lists {
		if len(l.Profiles) == 0 {
			rows = append(rows, []string{a.fit(l.Label, 40), a.txt(i18n.N(int64(l.Chats)))})
		}
	}
	if rows == nil {
		return
	}
	a.heading(i18n.T("cli.list.unowned"))
	a.print(a.out, a.table([]string{i18n.T("cli.list.list"), i18n.T("cli.list.chats")}, rows, 1))
}

func (a *app) switchTo(cmd *cobra.Command, args []string) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	name := args[0]
	if p, ok := env.Vault.Find(name); ok {
		name = p.Name
	}
	err = a.write(cmd.Context(), env, true, func() error { return env.Switch(name) })
	switch {
	case ops.Is(err, ops.AlreadyCurrent) && a.flags.json:
		return a.json(map[string]any{"profile": name, "changed": false})
	case ops.Is(err, ops.AlreadyCurrent):
		a.warn(a.out, i18n.T("cli.switch.already", "name", name))
		return nil
	case err != nil:
		return err
	}
	return a.result(map[string]any{"profile": name, "changed": true}, "cli.switch.done", "name", name)
}

// result prints a finished change: JSON when asked, else "✓ text".
func (a *app) result(v any, key string, args ...any) error {
	if a.flags.json {
		return a.json(v)
	}
	a.done(a.out, i18n.T(key, args...))
	return nil
}

func (a *app) save(cmd *cobra.Command, name string, replace bool) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	save := func() error { return env.Save(name, replace) }
	err = a.write(cmd.Context(), env, false, save)
	if ops.Is(err, ops.OtherAccount) && !replace {
		if err := a.confirm(cmd.Context(), i18n.T("cli.save.replace", "name", name)); err != nil {
			return err
		}
		replace = true
		err = a.write(cmd.Context(), env, false, save)
	}
	if err != nil {
		return err
	}
	return a.result(map[string]any{"profile": name}, "cli.save.done", "name", name)
}

func (a *app) add(cmd *cobra.Command, args []string) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	if err := a.write(cmd.Context(), env, true, func() error { return env.Add(args[0]) }); err != nil {
		return err
	}
	return a.result(map[string]any{"profile": args[0]}, "cli.add.done", "name", args[0])
}

func (a *app) rename(_ *cobra.Command, args []string) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	if err := env.Rename(args[0], args[1]); err != nil {
		return err
	}
	return a.result(map[string]any{"old": args[0], "profile": args[1]}, "cli.rename.done", "old", args[0], "name", args[1])
}

func (a *app) remove(cmd *cobra.Command, args []string) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	// Refuse before asking, so the question is only put when the answer matters.
	p, ok := env.Vault.Find(args[0])
	switch {
	case !ok:
		return &ops.Error{Code: ops.NoProfile, Args: []any{"name", args[0]}}
	case p.Name == env.Vault.Current():
		return &ops.Error{Code: ops.ProfileInUse, Args: []any{"name", p.Name}}
	}
	if err := a.confirm(cmd.Context(), i18n.T("cli.remove.confirm", "name", p.Name)); err != nil {
		return err
	}
	if err := env.Remove(p.Name); err != nil {
		return err
	}
	return a.result(map[string]any{"profile": p.Name}, "cli.remove.done", "name", p.Name)
}
