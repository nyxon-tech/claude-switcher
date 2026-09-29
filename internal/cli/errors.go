package cli

import (
	"context"
	"errors"
	"io"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

// Exit codes, documented in the root help.
const (
	exitOK        = 0
	exitError     = 1
	exitUsage     = 2
	exitRefused   = 3 // a safety check said no
	exitNotFound  = 4
	exitCancelled = 5
)

// cliError gives an error an exit code (0 keeps the one of the error it wraps) and a hint line.
type cliError struct {
	err  error
	code int
	hint string
}

func (e *cliError) Error() string { return e.err.Error() }
func (e *cliError) Unwrap() error { return e.err }

func errText(key string, args ...any) error { return errors.New(i18n.T(key, args...)) }

func usageError(err error, c *cobra.Command) error {
	return &cliError{err: err, code: exitUsage, hint: i18n.T("cli.hint.help", "command", c.CommandPath())}
}

func cancelled() error { return &cliError{err: errText("cli.cancelled"), code: exitCancelled} }

var opsExit = map[ops.Code]int{
	ops.DesktopRunning: exitRefused, ops.OtherAccount: exitRefused, ops.ProfileInUse: exitRefused,
	ops.NotSignedIn: exitRefused, ops.NoCurrent: exitRefused, ops.ProfileExists: exitRefused,
	ops.NoInstall: exitNotFound, ops.NoProfile: exitNotFound, ops.NoList: exitNotFound,
	ops.ChatNotFound: exitNotFound, ops.NoHistory: exitNotFound,
	ops.BadName: exitUsage, ops.BadFormat: exitUsage, ops.SameList: exitUsage,
	ops.AmbiguousList: exitUsage, ops.AmbiguousChat: exitUsage,
}

func exitCode(err error) int {
	var ce *cliError
	var oe *ops.Error
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &ce) && ce.code != 0:
		return ce.code
	case errors.Is(err, context.Canceled):
		return exitCancelled
	case errors.As(err, &oe) && opsExit[oe.Code] != 0:
		return opsExit[oe.Code]
	}
	return exitError
}

// printError writes "✗ message" and a hint line to stderr (Fang's error handler).
func (a *app) printError(_ io.Writer, _ fang.Styles, err error) {
	if errors.Is(err, context.Canceled) {
		err = cancelled()
	}
	a.say(a.err, a.st.fail.Render("✗"), a.st.fail, err.Error())
	var ce *cliError
	if errors.As(err, &ce) && ce.hint != "" {
		a.note(a.err, ce.hint)
	}
}
