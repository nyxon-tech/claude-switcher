package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

type checkJSON struct {
	Level  ops.Level `json:"level"`
	Title  string    `json:"title"`
	Detail string    `json:"detail"`
	Fix    string    `json:"fix"`
}

// doctor prints every check and fails (exit 1) when one of them failed.
func (a *app) doctor(*cobra.Command, []string) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	checks := env.Doctor()
	attention, failed := 0, false
	for _, c := range checks {
		if c.Level == ops.Warn || c.Level == ops.Fail {
			attention++
		}
		failed = failed || c.Level == ops.Fail
	}
	if a.flags.json {
		out := make([]checkJSON, 0, len(checks))
		for _, c := range checks {
			out = append(out, checkJSON(c))
		}
		if err := a.json(out); err != nil {
			return err
		}
	} else {
		a.printChecks(checks, attention)
	}
	if failed {
		return &cliError{err: errText("cli.doctor.failed"), code: exitError}
	}
	return nil
}

func (a *app) printChecks(checks []ops.Check, attention int) {
	if attention == 0 {
		a.done(a.out, i18n.T("cli.doctor.fine"))
	} else {
		a.warn(a.out, i18n.T("cli.doctor.attention", "n", attention))
	}
	icons := map[ops.Level]string{ops.OK: a.st.ok.Render("✓"), ops.Info: a.st.accent.Render("i"),
		ops.Warn: a.st.warn.Render("!"), ops.Fail: a.st.fail.Render("✗")}
	for _, c := range checks {
		fmt.Fprintln(a.out)
		a.say(a.out, icons[c.Level], a.st.plain, c.Title)
		if c.Detail != "" {
			a.note(a.out, c.Detail)
		}
		if c.Fix != "" {
			a.say(a.out, a.line(" ", a.pointer()), a.st.accent, c.Fix)
		}
	}
}
