package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/ops"
)

func (a *app) exportCommand() *cobra.Command {
	var format, output string
	cmd := a.command("export <chat id>", "export", "chats", nargs(1, 1), func(_ *cobra.Command, args []string) error {
		return a.export(args[0], format, output)
	})
	cmd.Flags().StringVar(&format, "format", "html", a.txt(i18n.T("cli.flag.format")))
	cmd.Flags().StringVarP(&output, "output", "o", "", a.txt(i18n.T("cli.flag.output")))
	return cmd
}

func (a *app) export(ref, format, output string) error {
	if format != "html" && format != "md" {
		return &ops.Error{Code: ops.BadFormat, Args: []any{"format", format}}
	}
	env, err := a.open()
	if err != nil {
		return err
	}
	c, err := a.findChat(env, ref)
	if err != nil {
		return err
	}
	if output == "-" {
		return env.Export(c, format, a.stdout)
	}
	path := output
	if path == "" {
		path = freeName(fileName(c), "."+format)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = env.Export(c, format, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return err
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return a.result(map[string]any{"path": path, "chat": c.ID, "format": format}, "cli.export.done", "path", path)
}

// findChat finds the chat ref names in any list: its id, the id without "local_", its
// transcript id, or a prefix of one of them. A chat held by several lists is their newest copy.
func (a *app) findChat(env *ops.Env, ref string) (ops.Chat, error) {
	lists, err := env.Lists()
	if err != nil {
		return ops.Chat{}, err
	}
	chats, err := allChats(env, lists)
	if err != nil {
		return ops.Chat{}, err
	}
	var hits []ops.Chat
	seen := map[string]bool{}
	for _, c := range chats {
		if c.ID == ref || c.Session == ref {
			return c, nil
		}
		if !seen[c.ID] && (prefixFold(c.ID, ref) || prefixFold(c.ID, "local_"+ref) || c.Session != "" && prefixFold(c.Session, ref)) {
			seen[c.ID] = true
			hits = append(hits, c)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return ops.Chat{}, &ops.Error{Code: ops.ChatNotFound, Args: []any{"ref", ref}}
	}
	return ops.Chat{}, &ops.Error{Code: ops.AmbiguousChat, Args: []any{"ref", ref, "n", len(hits)}}
}

func prefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// fileName makes a chat's title safe as a file name on every OS.
func fileName(c ops.Chat) string {
	name := strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, c.Title)
	name = strings.Trim(string([]rune(name)[:min(80, len([]rune(name)))]), " .")
	if name == "" {
		return "chat-" + shortID(c.ID)
	}
	return name
}

// freeName is base+ext, numbered when a file of that name exists already.
func freeName(base, ext string) string {
	name := base + ext
	for i := 2; exists(name); i++ {
		name = fmt.Sprintf("%s (%d)%s", base, i, ext)
	}
	return name
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
