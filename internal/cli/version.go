package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ui"
	"github.com/nyxon-tech/claude-switcher/v3/internal/update"
)

func (a *app) moreCommands() []*cobra.Command {
	var short bool
	version := a.command("version", "version", "more", nargs(0, 0), func(*cobra.Command, []string) error {
		return a.version(short)
	})
	version.Flags().BoolVar(&short, "short", false, a.txt(i18n.T("cli.flag.short")))
	return []*cobra.Command{
		a.usageCommand(),
		a.command("doctor", "doctor", "more", nargs(0, 0), a.doctor),
		a.configCommand(),
		version,
		a.command("update", "update", "more", nargs(0, 0), a.update),
		a.repairCommand(),
	}
}

// installMethod is how this executable was installed; a development build says so.
func (a *app) installMethod() string {
	if a.build.Version == "dev" {
		return "source"
	}
	exe, err := os.Executable()
	if err != nil {
		return string(update.Installer)
	}
	return string(update.MethodOf(exe))
}

func (a *app) version(short bool) error {
	b := a.build
	switch {
	case short:
		_, err := fmt.Fprintln(a.stdout, b.Version)
		return err
	case a.flags.json:
		return a.json(map[string]string{"version": b.Version, "commit": b.Commit, "date": b.Date,
			"os": runtime.GOOS, "arch": runtime.GOARCH, "install": a.installMethod(), "go": runtime.Version()})
	}
	if a.tty {
		a.print(a.out, ui.Logo(a.st.pal))
		fmt.Fprintln(a.out)
	}
	a.print(a.out, a.line(a.st.bold.Render(a.txt(i18n.T("app.name"))), a.st.accent.Render(b.Version), a.st.muted.Render(a.txt(i18n.T("app.by")))))
	date := b.Date
	if t, err := time.Parse(time.RFC3339, b.Date); err == nil {
		date = i18n.Date(t.Local())
	}
	if date != "" {
		date = a.txt(date)
	}
	a.pairs([][2]string{
		{i18n.T("cli.version.commit"), b.Commit},
		{i18n.T("cli.version.built"), date},
		{i18n.T("cli.version.platform"), runtime.GOOS + "/" + runtime.GOARCH},
		{i18n.T("cli.version.install"), a.txt(i18n.T("cli.install." + a.installMethod()))},
	})
	return nil
}

// upgradeCommand is what updates this installation.
func upgradeCommand() string {
	if exe, err := os.Executable(); err == nil {
		if cmd := update.MethodOf(exe).Upgrade(); cmd != "" {
			return cmd
		}
	}
	return "claude-switcher update"
}

func (a *app) update(cmd *cobra.Command, _ []string) error {
	current := a.build.Version
	if current == "dev" {
		return errText("cli.err.dev-build")
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe) // replace the file, not a link to it
	}
	if err != nil {
		return err
	}
	switch m := update.MethodOf(exe); {
	case m.Upgrade() != "":
		return a.result(map[string]any{"current": current, "method": m, "command": m.Upgrade()},
			"cli.update.managed", "method", i18n.T("cli.install."+string(m)), "command", m.Upgrade())
	case m == update.Package:
		url := update.Releases + "/latest"
		return a.result(map[string]any{"current": current, "method": m, "url": url}, "cli.update.package", "url", url)
	}
	ctx := cmd.Context()
	var latest string
	err = a.spin(ctx, func() string { return i18n.T("cli.update.checking") }, func(ctx context.Context) (err error) {
		latest, err = update.Latest(ctx, http.DefaultClient)
		return err
	})
	if err != nil {
		return err
	}
	if !update.Newer(latest, current) {
		return a.result(map[string]any{"current": current, "latest": latest, "updated": false}, "cli.update.latest", "version", current)
	}
	err = a.spin(ctx, func() string { return i18n.T("cli.update.downloading", "version", latest) }, func(ctx context.Context) error {
		return update.Install(ctx, http.DefaultClient, exe)
	})
	if err != nil {
		return err
	}
	return a.result(map[string]any{"current": current, "latest": latest, "updated": true}, "cli.update.done", "version", latest)
}
