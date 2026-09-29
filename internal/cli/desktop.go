package cli

import (
	"context"
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
)

// unattendedWait is how long a run without a terminal waits for Desktop to close before it
// refuses, so a script never hangs.
var unattendedWait = 30 * time.Second

func (a *app) forceQuitFlag(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().BoolVar(&a.flags.forceQuit, "force-quit", false, a.txt(i18n.T("cli.flag.force-quit")))
	return cmd
}

// write makes a change that needs Claude Desktop closed. ops checks everything else first and
// refuses with desktop-running only for a change that would otherwise go ahead, so the user is
// asked to quit Desktop only when it is worth it. Once the command has reported, Desktop starts
// again when it ran before, or always when launch is set (switch and add), unless --no-launch.
func (a *app) write(ctx context.Context, env *ops.Env, launch bool, change func() error) error {
	err := change()
	wasRunning := ops.Is(err, ops.DesktopRunning)
	if wasRunning {
		if err = a.closeDesktop(ctx, env); err != nil {
			return err
		}
		err = change()
	}
	if !a.flags.noLaunch && (wasRunning || launch && err == nil) {
		a.relaunch = env
	}
	return err
}

// closeDesktop gets Desktop closed: --force-quit ends it, macOS and Linux are asked to quit,
// and on Windows the user quits it from the tray while a spinner waits. Ctrl+C gives up with
// nothing changed.
func (a *app) closeDesktop(ctx context.Context, env *ops.Env) error {
	if !a.ttyIn {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, unattendedWait)
		defer cancel()
	}
	if a.flags.forceQuit {
		a.note(a.err, i18n.T("cli.desktop.force-quitting"))
		if err := env.Desktop.ForceQuit(); err != nil {
			return err
		}
	} else {
		switch err := env.Desktop.Quit(); {
		case errors.Is(err, platform.ErrManualQuit):
			a.warn(a.err, i18n.T("cli.desktop.quit-tray"))
		case err != nil:
			a.warn(a.err, i18n.T("cli.desktop.quit-yourself"))
		default:
			a.note(a.err, i18n.T("cli.desktop.quitting"))
		}
		a.note(a.err, i18n.T("cli.desktop.wait-hint"))
	}
	err := a.spin(ctx, func() string { return i18n.T("cli.desktop.waiting") }, env.WaitClosed)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &cliError{err: &ops.Error{Code: ops.DesktopRunning}, hint: i18n.T("cli.hint.force-quit")}
	case errors.Is(err, context.Canceled):
		return cancelled()
	case err == nil:
		a.done(a.err, i18n.T("ops.doctor.closed"))
	}
	return err
}

// launch starts Desktop again after a change asked for it.
func (a *app) launch() {
	env := a.relaunch
	if env == nil || env.Install.Kind == platform.Custom {
		return // a data folder given by hand has no app to start
	}
	if err := env.Desktop.Launch(); err != nil {
		a.warn(a.err, i18n.T("cli.desktop.launch-failed"))
		return
	}
	a.done(a.err, i18n.T("cli.desktop.launched"))
}
