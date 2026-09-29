package cli

import (
	"cmp"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

// setting is one key of config: its name on the command line and in JSON, the values it takes
// and how it maps to settings.json.
type setting struct {
	key, json string
	values    []string
	get       func(store.Settings) string
	set       func(*store.Settings, string)
}

var onOff = []string{"on", "off"}

func toggle(key, json string, field func(*store.Settings) *bool) setting {
	return setting{key, json, onOff,
		func(s store.Settings) string { return map[bool]string{true: "on", false: "off"}[*field(&s)] },
		func(s *store.Settings, v string) { *field(s) = v == "on" }}
}

var settings = []setting{
	{"theme", "theme", themeNames,
		func(s store.Settings) string { return shortTheme(s.Theme) },
		func(s *store.Settings, v string) { s.Theme = longTheme(v) }},
	{"rtl", "rtl", []string{"auto", "app", "terminal", "off"},
		func(s store.Settings) string { return cmp.Or(s.RTL, "auto") },
		func(s *store.Settings, v string) { s.RTL = v }},
	toggle("update-check", "updateCheck", func(s *store.Settings) *bool { return &s.UpdateCheck }),
}

func (a *app) configCommand() *cobra.Command {
	list := func(*cobra.Command, []string) error { return a.configList() }
	cmd := a.command("config", "config", "more", nargs(0, 0), list)
	cmd.AddCommand(
		a.command("list", "config-list", "", nargs(0, 0), list),
		a.command("get <key>", "config-get", "", nargs(1, 1), func(_ *cobra.Command, args []string) error {
			s, err := findSetting(args[0])
			if err != nil {
				return err
			}
			v := s.get(a.settings)
			if a.flags.json {
				return a.json(map[string]any{s.json: jsonValue(s, v)})
			}
			_, err = a.out.Write([]byte(v + "\n"))
			return err
		}),
		a.command("set <key> <value>", "config-set", "", nargs(2, 2), func(_ *cobra.Command, args []string) error {
			return a.configSet(args[0], args[1])
		}),
	)
	return cmd
}

func findSetting(key string) (setting, error) {
	i := slices.IndexFunc(settings, func(s setting) bool { return s.key == key })
	if i < 0 {
		keys := make([]string, len(settings))
		for i, s := range settings {
			keys[i] = s.key
		}
		return setting{}, &cliError{err: errText("cli.err.config-key", "key", key, "keys", strings.Join(keys, ", ")), code: exitUsage}
	}
	return settings[i], nil
}

func jsonValue(s setting, v string) any {
	if slices.Equal(s.values, onOff) {
		return v == "on"
	}
	return v
}

func (a *app) configList() error {
	if a.flags.json {
		out := map[string]any{}
		for _, s := range settings {
			out[s.json] = jsonValue(s, s.get(a.settings))
		}
		return a.json(out)
	}
	var rows [][]string
	for _, s := range settings {
		rows = append(rows, []string{s.key, a.st.accent.Render(s.get(a.settings))})
	}
	a.print(a.out, a.table([]string{i18n.T("cli.config.key"), i18n.T("cli.config.value")}, rows))
	return nil
}

func (a *app) configSet(key, value string) error {
	s, err := findSetting(key)
	if err != nil {
		return err
	}
	switch strings.ToLower(value) {
	case "true", "yes", "1":
		value = "on"
	case "false", "no", "0":
		value = "off"
	}
	if !slices.Contains(s.values, value) {
		return &cliError{err: errText("cli.err.config-value", "key", key, "value", value, "allowed", strings.Join(s.values, ", ")), code: exitUsage}
	}
	// Read again: a settings file that cannot be read must not be replaced by the defaults.
	saved, err := a.vault.Settings()
	if err != nil {
		return err
	}
	s.set(&saved, value)
	if err := a.vault.SaveSettings(saved); err != nil {
		return err
	}
	return a.result(map[string]any{s.json: jsonValue(s, value)}, "cli.config.set", "key", key, "value", value)
}
