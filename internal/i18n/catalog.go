package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"strings"
)

//go:embed locales
var localeFS embed.FS

var catalogs = loadCatalogs(localeFS)

// rules are what a language needs beyond its text. A language missing from langRules writes
// like English.
type rules struct {
	rtl         bool
	zero        rune // the language's digit zero; 0 means ASCII digits and signs
	group       rune // thousands separator used with the language's digits
	decimal     rune // decimal separator used with the language's digits
	percent     rune // percent sign used with the language's digits
	jalali      bool // dates use the Jalali calendar unless turned off
	alwaysOther bool // nouns stay singular after numbers, so ".one" is never used
}

var langRules = map[string]rules{
	"fa": {rtl: true, zero: '۰', group: '٬', decimal: '٫', percent: '٪', jalali: true, alwaysOther: true},
}

// catalog is one language's text and rules.
type catalog struct {
	lang     string
	text     map[string]string
	rules    rules
	fallback *catalog // English, for keys this language lacks
}

// loadCatalogs merges locales/<lang>/*.json per language. A broken or clashing file is a build
// mistake that every test run catches, so it panics.
func loadCatalogs(fsys fs.FS) map[string]*catalog {
	files, err := fs.Glob(fsys, "locales/*/*.json")
	if err != nil {
		panic(err)
	}
	cats := map[string]*catalog{}
	for _, name := range files {
		lang := path.Base(path.Dir(name))
		c := cats[lang]
		if c == nil {
			c = &catalog{lang: lang, text: map[string]string{}, rules: langRules[lang]}
			cats[lang] = c
		}
		if err := c.add(fsys, name); err != nil {
			panic("i18n: " + err.Error())
		}
	}
	en := cats["en"]
	if en == nil {
		panic("i18n: no English catalog")
	}
	for _, c := range cats {
		if c != en {
			c.fallback = en
		}
	}
	return cats
}

func (c *catalog) add(fsys fs.FS, name string) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	var text map[string]string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for k, v := range text {
		if _, dup := c.text[k]; dup {
			return fmt.Errorf("%s: key %q is already defined in another file", name, k)
		}
		c.text[k] = v
	}
	return nil
}

func (c *catalog) t(key string, args ...any) string {
	n, counted := count(args)
	for from := c; from != nil; from = from.fallback {
		if text, ok := from.lookup(key, n, counted); ok {
			return c.fill(text, args)
		}
	}
	return key
}

func (c *catalog) lookup(key string, n int64, counted bool) (string, bool) {
	if counted {
		form := ".other"
		if n == 1 && !c.rules.alwaysOther {
			form = ".one"
		}
		if text, ok := c.text[key+form]; ok {
			return text, true
		}
	}
	text, ok := c.text[key]
	return text, ok
}

func (c *catalog) fill(text string, args []any) string {
	if len(args) < 2 || !strings.Contains(text, "{") {
		return text
	}
	pairs := make([]string, 0, len(args))
	for i := 0; i+1 < len(args); i += 2 {
		name, _ := args[i].(string)
		value := fmt.Sprint(args[i+1])
		if v, ok := integer(args[i+1]); ok {
			value = c.n(v)
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

// supported is the language of a locale name when it has a catalog, else "en".
func supported(locale string) string {
	if lang := baseLang(locale); catalogs[lang] != nil {
		return lang
	}
	return "en"
}

// baseLang is the language part of a locale name: "fa_IR.UTF-8" and "fa-IR" give "fa".
func baseLang(locale string) string {
	if i := strings.IndexAny(locale, "_-.@"); i >= 0 {
		locale = locale[:i]
	}
	return strings.ToLower(locale)
}
