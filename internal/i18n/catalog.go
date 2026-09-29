package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
)

//go:embed locales
var localeFS embed.FS

// english is the catalog T reads.
var english = load(localeFS)

// catalog is a language's text: keys to strings.
type catalog map[string]string

// load merges locales/en/*.json. A broken file, or a key defined in two files, is a build
// mistake that every test run catches, so it panics.
func load(fsys fs.FS) catalog {
	files, err := fs.Glob(fsys, "locales/en/*.json")
	if err != nil || len(files) == 0 {
		panic("i18n: no English catalog")
	}
	c := catalog{}
	for _, name := range files {
		if err := c.add(fsys, name); err != nil {
			panic("i18n: " + err.Error())
		}
	}
	return c
}

func (c catalog) add(fsys fs.FS, name string) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	var text map[string]string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for k, v := range text {
		if _, dup := c[k]; dup {
			return fmt.Errorf("%s: key %q is already defined in another file", name, k)
		}
		c[k] = v
	}
	return nil
}

func (c catalog) t(key string, args ...any) string {
	if text, ok := c.lookup(key, args); ok {
		return fill(text, args)
	}
	return key
}

func (c catalog) lookup(key string, args []any) (string, bool) {
	if n, counted := count(args); counted {
		form := ".other"
		if n == 1 {
			form = ".one"
		}
		if text, ok := c[key+form]; ok {
			return text, true
		}
	}
	text, ok := c[key]
	return text, ok
}

func fill(text string, args []any) string {
	if len(args) < 2 || !strings.Contains(text, "{") {
		return text
	}
	pairs := make([]string, 0, len(args))
	for i := 0; i+1 < len(args); i += 2 {
		name, _ := args[i].(string)
		value := fmt.Sprint(args[i+1])
		if v, ok := integer(args[i+1]); ok {
			value = N(v)
		}
		pairs = append(pairs, "{"+name+"}", value)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// count finds the argument "n". An n already written as text, such as Compact's "1.2k", still
// picks a form: "1" is one and anything else is many.
func count(args []any) (int64, bool) {
	for i := 0; i+1 < len(args); i += 2 {
		if args[i] != "n" {
			continue
		}
		if v, ok := integer(args[i+1]); ok {
			return v, true
		}
		if fmt.Sprint(args[i+1]) == "1" {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func integer(v any) (int64, bool) {
	rv := reflect.ValueOf(v)
	switch {
	case rv.CanInt():
		return rv.Int(), true
	case rv.CanUint():
		return int64(rv.Uint()), true
	}
	return 0, false
}
