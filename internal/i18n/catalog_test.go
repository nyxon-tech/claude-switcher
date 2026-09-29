package i18n

import (
	"bytes"
	"encoding/json"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"unicode"
)

var placeholder = regexp.MustCompile(`\{\w+\}`)

func readLocale(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := fs.ReadFile(localeFS, name)
	if err != nil {
		t.Fatal(err)
	}
	var text map[string]string
	if err := json.Unmarshal(data, &text); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if dups := duplicateKeys(data); len(dups) > 0 {
		t.Errorf("%s defines %q more than once; only the last one is kept", name, dups)
	}
	return text
}

// duplicateKeys lists the keys a flat JSON object repeats, which json.Unmarshal lets through.
func duplicateKeys(data []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(data))
	seen := map[string]bool{}
	var dups []string
	isKey := true // strings alternate key, value
	for {
		tok, err := dec.Token()
		if err != nil {
			return dups
		}
		s, ok := tok.(string)
		if !ok {
			continue
		}
		if isKey {
			if seen[s] {
				dups = append(dups, s)
			}
			seen[s] = true
		}
		isKey = !isKey
	}
}

func placeholders(s string) []string {
	found := placeholder.FindAllString(s, -1)
	slices.Sort(found)
	return slices.Compact(found)
}

// Every text is English with well-formed placeholders, no file repeats a key, and counted text
// has both forms, with the same placeholders but for {n} ("Last day", "Last {n} days").
func TestCatalog(t *testing.T) {
	files, _ := fs.Glob(localeFS, "locales/en/*.json")
	for _, name := range files {
		readLocale(t, name)
	}
	withoutN := func(s string) []string {
		return slices.DeleteFunc(placeholders(s), func(p string) bool { return p == "{n}" })
	}
	for k, v := range english {
		if strings.Count(v, "{") != len(placeholder.FindAllString(v, -1)) || strings.Count(v, "}") != strings.Count(v, "{") {
			t.Errorf("%q has a broken placeholder: %q", k, v)
		}
		if strings.ContainsFunc(v, func(r rune) bool { return unicode.In(r, unicode.Arabic, unicode.Hebrew) }) {
			t.Errorf("%q is not English: %q", k, v)
		}
		if base, ok := strings.CutSuffix(k, ".other"); ok {
			if one, ok := english[base+".one"]; !ok {
				t.Errorf("%q has no .one form", base)
			} else if !slices.Equal(withoutN(v), withoutN(one)) {
				t.Errorf("%q has placeholders %v in one form and %v in the other", base, placeholders(one), placeholders(v))
			}
		}
		if base, ok := strings.CutSuffix(k, ".one"); ok && english[base+".other"] == "" {
			t.Errorf("%q has no .other form", base)
		}
	}
}

// keyLike is a string literal that reads like a catalog key.
var keyLike = regexp.MustCompile(`^[a-z]+(\.[a-z0-9_-]+)+$`)

// Every key the code names exists: each string literal in the module's Go files (tests aside)
// that reads like a key of a namespace the catalog has, as itself or as counted text.
func TestKeysInCodeExist(t *testing.T) {
	namespaces := map[string]bool{}
	for k := range english {
		namespaces[strings.SplitN(k, ".", 2)[0]] = true
	}
	notKeys := map[string]bool{"settings.json": true} // a file name
	found := 0
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && path != root && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")):
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go"):
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var s scanner.Scanner
		s.Init(token.NewFileSet().AddFile(path, -1, len(src)), src, nil, 0)
		for {
			_, tok, lit := s.Scan()
			if tok == token.EOF {
				return nil
			}
			key, err := strconv.Unquote(lit)
			if tok != token.STRING || err != nil || !keyLike.MatchString(key) || notKeys[key] ||
				!namespaces[strings.SplitN(key, ".", 2)[0]] {
				continue
			}
			found++
			if _, ok := english[key]; !ok {
				if _, ok := english[key+".other"]; !ok {
					t.Errorf("%s names %q, which is not in locales/en", path, key)
				}
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if found < 100 {
		t.Errorf("only %d keys found in the code; is the walk looking in the right place?", found)
	}
}

func TestDuplicateKeys(t *testing.T) {
	got := duplicateKeys([]byte(`{"a": "b", "b": "x", "a": "y"}`))
	if !slices.Equal(got, []string{"a"}) {
		t.Errorf("duplicateKeys = %q, want [a]", got)
	}
}

func TestLoadMergesFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"locales/en/common.json": {Data: []byte(`{"a": "A"}`)},
		"locales/en/ops.json":    {Data: []byte(`{"b": "B"}`)},
	}
	if c := load(fsys); c.t("a") != "A" || c.t("b") != "B" {
		t.Errorf("load = %v, want a and b from both files", c)
	}

	fsys["locales/en/ops.json"] = &fstest.MapFile{Data: []byte(`{"a": "again"}`)}
	defer func() {
		if recover() == nil {
			t.Error("a key defined in two files should panic")
		}
	}()
	load(fsys)
}
