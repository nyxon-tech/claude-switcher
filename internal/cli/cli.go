// Package cli is Claude Switcher's command line: every action of the app as a command, with
// styled help (Fang), --json output and stable exit codes. Run without a command in a terminal,
// it opens the full-screen app.
package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"charm.land/fang/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
	"github.com/nyxon-tech/claude-switcher/v3/internal/platform"
	"github.com/nyxon-tech/claude-switcher/v3/internal/rtl"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ui"
	"github.com/nyxon-tech/claude-switcher/v3/internal/update"
)

// Build is what the binary knows about itself.
type Build struct{ Version, Commit, Date string }

// Execute runs the command line on the process's arguments and returns the exit code.
func Execute(b Build) int {
	return run(context.Background(), b, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

// openEnv opens the folders a command works on. Tests swap it to fake Claude Desktop.
var openEnv = ops.Open

func run(ctx context.Context, b Build, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if exe, err := os.Executable(); err == nil {
		update.Cleanup(exe)
	}
	return newApp(b, args, stdin, stdout, stderr).execute(ctx, args)
}

func (a *app) execute(ctx context.Context, args []string) int {
	root := a.root()
	root.SetArgs(args)
	root.SetIn(a.in)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	err := fang.Execute(ctx, root,
		fang.WithVersion(a.build.Version), fang.WithCommit(a.build.Commit),
		fang.WithColorSchemeFunc(a.helpColors), fang.WithErrorHandler(a.printError),
		fang.WithNotifySignal(os.Interrupt))
	a.launch()
	a.notifyUpdate()
	return exitCode(err)
}

// globals are the flags every command takes.
type globals struct {
	json, yes, noLaunch, forceQuit bool
	lang, rtl, theme               string
	dataDir, vaultDir, projectsDir string
}

func (g *globals) register(fs *pflag.FlagSet, usage func(key string) string) {
	fs.BoolVar(&g.json, "json", false, usage("cli.flag.json"))
	fs.BoolVarP(&g.yes, "yes", "y", false, usage("cli.flag.yes"))
	fs.BoolVar(&g.noLaunch, "no-launch", false, usage("cli.flag.no-launch"))
	fs.StringVar(&g.lang, "lang", "", usage("cli.flag.lang"))
	fs.StringVar(&g.rtl, "rtl", "", usage("cli.flag.rtl"))
	fs.StringVar(&g.theme, "theme", "", usage("cli.flag.theme"))
	for name, p := range map[string]*string{"data-dir": &g.dataDir, "vault-dir": &g.vaultDir, "projects-dir": &g.projectsDir} {
		fs.StringVar(p, name, "", "")
		_ = fs.MarkHidden(name)
	}
}

// app is one run of the command line.
type app struct {
	build              Build
	flags              globals
	in                 io.Reader
	stdout, stderr     io.Writer // raw: JSON and exports, and what Fang sets up for colour
	out, err           io.Writer // text, with colour only where the terminal shows it
	tty, ttyIn, ttyErr bool
	mode               rtl.Mode
	mirror             bool // Persian laid out by us: lines read right to left
	width              int
	st                 styles
	vault              store.Vault
	settings           store.Settings
	env                *ops.Env
	relaunch           *ops.Env    // start Desktop again once the command has reported
	newer              chan string // the update check started for this command
}

// newApp reads the global flags ahead of Cobra, because the language decides every help text.
func newApp(b Build, args []string, stdin io.Reader, stdout, stderr io.Writer) *app {
	a := &app{build: b, in: stdin, stdout: stdout, stderr: stderr,
		out: colorprofile.NewWriter(stdout, os.Environ()), err: colorprofile.NewWriter(stderr, os.Environ()),
		tty: isTerminal(stdout), ttyIn: isTerminal(stdin), ttyErr: isTerminal(stderr), width: maxWidth}
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.ParseErrorsAllowlist.UnknownFlags = true
	fs.SetOutput(io.Discard)
	fs.BoolP("help", "h", false, "")
	a.flags.register(fs, func(string) string { return "" })
	_ = fs.Parse(args) // Cobra reports bad flags

	a.vault = store.Vault{Dir: cmp.Or(a.flags.vaultDir, platform.VaultDir())}
	a.settings, _ = a.vault.Settings() // a broken settings file reads as the defaults
	i18n.Load(cmp.Or(a.flags.lang, a.settings.Lang, i18n.Detect()))
	i18n.SetPersianDigits(a.settings.PersianDigits)
	i18n.SetJalali(a.settings.Jalali)

	a.mode = rtl.Off
	if a.tty {
		a.mode, _ = rtl.ParseMode(cmp.Or(a.flags.rtl, a.settings.RTL))
		if a.mode == rtl.Auto {
			a.mode = rtl.Detect(os.Getenv, runtime.GOOS)
		}
		if w, _, err := term.GetSize(stdout.(term.File).Fd()); err == nil && w > 0 {
			a.width = min(w, maxWidth)
		}
	}
	a.mirror = i18n.RTL() && a.mode == rtl.App
	a.st = newStyles(ui.ThemePalette(a.theme(), a.darkBackground()))
	return a
}

// maxWidth caps how wide output grows in a wide window, and is the width piped output lays out at.
const maxWidth = 160

func isTerminal(v any) bool {
	f, ok := v.(term.File)
	return ok && term.IsTerminal(f.Fd())
}

// root builds the command tree in the loaded language.
func (a *app) root() *cobra.Command {
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:               "claude-switcher",
		Short:             a.txt(i18n.T("cli.root.short")),
		Long:              a.para(i18n.T("cli.root.long"), a.width-4), // Fang indents it by two
		Example:           a.examples(),
		Args:              a.noCommand,
		PersistentPreRunE: a.prepare,
		RunE:              a.openApp,
	}
	a.flags.register(root.PersistentFlags(), func(key string) string { return a.txt(i18n.T(key)) })
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error { return usageError(err, c) })
	for _, id := range []string{"accounts", "chats", "more"} {
		root.AddGroup(&cobra.Group{ID: id, Title: a.txt(i18n.T("cli.group." + id))})
	}
	root.SetHelpCommandGroupID("more")
	root.SetCompletionCommandGroupID("more")
	root.AddCommand(a.accountCommands()...)
	root.AddCommand(a.chatCommands()...)
	root.AddCommand(a.moreCommands()...)
	a.localizeBuiltins(root)
	return root
}

// localizeBuiltins translates what Cobra adds by itself: the help and completion commands and
// the --help and --version flags.
func (a *app) localizeBuiltins(root *cobra.Command) {
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	root.Flags().BoolP("version", "v", false, a.txt(i18n.T("cli.flag.version")))
	for _, c := range root.Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			c.Short = a.txt(i18n.T("cli.cmd." + c.Name()))
		}
	}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		c.Flags().Lookup("help").Usage = a.txt(i18n.T("cli.flag.help"))
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

// nargs accepts between lo and hi arguments (hi < 0: any number).
func nargs(lo, hi int) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if len(args) < lo || hi >= 0 && len(args) > hi {
			return usageError(errText("cli.err.args", "usage", c.UseLine()), c)
		}
		return nil
	}
}

// command builds a command whose Short text is the i18n key cli.cmd.<key>.
func (a *app) command(use, key, group string, args cobra.PositionalArgs, run func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: a.txt(i18n.T("cli.cmd." + key)), GroupID: group, Args: args, RunE: run}
}

func (a *app) examples() string {
	var b strings.Builder
	for _, ex := range []struct{ key, cmd string }{
		{"open", "claude-switcher"},
		{"save", "claude-switcher save work\nclaude-switcher add personal"},
		{"switch", "claude-switcher switch work"},
		{"copy", "claude-switcher copy --from work --to personal 1a2b3c4d"},
	} {
		b.WriteString("# " + a.txt(i18n.T("cli.example."+ex.key)) + "\n" + ex.cmd + "\n")
	}
	return b.String()
}

func (a *app) noCommand(c *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError(errText("cli.err.unknown-command", "name", args[0]), c)
	}
	return nil
}

// prepare checks the global flags and starts the daily update check for the command that runs.
func (a *app) prepare(cmd *cobra.Command, _ []string) error {
	for _, f := range []struct {
		name, value string
		allowed     []string
	}{
		{"lang", a.flags.lang, []string{"en", "fa"}},
		{"rtl", a.flags.rtl, []string{"auto", "app", "terminal", "off"}},
		{"theme", a.flags.theme, themeNames},
	} {
		if f.value != "" && !slices.Contains(f.allowed, f.value) {
			return usageError(errText("cli.err.flag-value", "flag", f.name, "value", f.value, "allowed", strings.Join(f.allowed, ", ")), cmd)
		}
	}
	if a.ttyErr && !a.flags.json && !quiet(cmd) {
		n := a.notifier()
		a.newer = make(chan string, 1)
		go func() { a.newer <- n.Newer(context.Background()) }()
	}
	return nil
}

// quiet commands never mention a new version: the app shows it itself, and version and update
// are about versions already.
func quiet(cmd *cobra.Command) bool {
	if !cmd.HasParent() {
		return true
	}
	for c := cmd; c.HasParent(); c = c.Parent() {
		if slices.Contains([]string{"version", "update", "completion", "man", "help", "__complete"}, c.Name()) {
			return true
		}
	}
	return false
}

func (a *app) notifier() update.Notifier {
	return update.Notifier{Current: a.build.Version, Cache: filepath.Join(a.vault.Dir, "cache", "update.json"),
		Off: !a.settings.UpdateCheck || update.Disabled(os.Getenv)}
}

// notifyUpdate says once, after the command, that a newer version is out.
func (a *app) notifyUpdate() {
	if a.newer == nil {
		return
	}
	if v := <-a.newer; v != "" {
		fmt.Fprintln(a.err)
		a.warn(a.err, i18n.T("cli.update.available", "version", v, "command", upgradeCommand()))
	}
}

// open finds Claude Desktop and the folders around it, once per run.
func (a *app) open() (*ops.Env, error) {
	if a.env == nil {
		env, err := openEnv(ops.Options{DataDir: a.flags.dataDir, VaultDir: a.vault.Dir, ProjectsDir: a.flags.projectsDir})
		if err != nil {
			return nil, err
		}
		a.env = env
	}
	return a.env, nil
}

// openApp is the root command: the full-screen app in a terminal, the help otherwise.
func (a *app) openApp(cmd *cobra.Command, _ []string) error {
	if !a.tty || !a.ttyIn {
		return cmd.Help()
	}
	env, err := a.open()
	if err != nil {
		return err
	}
	n := a.notifier()
	return ui.Run(env, ui.Options{Version: a.build.Version, Mode: a.mode, Theme: a.theme(), NoLaunch: a.flags.noLaunch,
		NewerVersion: func() string { return n.Newer(context.Background()) }})
}
