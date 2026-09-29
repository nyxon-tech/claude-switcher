package i18n

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
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

// Every language has the same files, keys and placeholders as English.
func TestCatalogsMatchEnglish(t *testing.T) {
	dirs, err := fs.ReadDir(localeFS, "locales")
	if err != nil {
		t.Fatal(err)
	}
	enFiles, _ := fs.Glob(localeFS, "locales/en/*.json")
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		lang := dir.Name()
		files, _ := fs.Glob(localeFS, "locales/"+lang+"/*.json")
		for _, name := range files {
			if !slices.Contains(enFiles, "locales/en/"+path.Base(name)) {
				t.Errorf("%s has no English original", name)
			}
		}
		for _, enName := range enFiles {
			name := "locales/" + lang + "/" + path.Base(enName)
			if !slices.Contains(files, name) {
				t.Errorf("%s is missing", name)
				continue
			}
			compareLocale(t, enName, name, langRules[lang])
		}
	}
}

func compareLocale(t *testing.T, enName, name string, r rules) {
	en, other := readLocale(t, enName), readLocale(t, name)
	for k, v := range en {
		w, ok := other[k]
		switch {
		case !ok && strings.HasSuffix(k, ".one") && r.alwaysOther:
		case !ok:
			t.Errorf("%s: missing %q", name, k)
		case w == "":
			t.Errorf("%s: %q is empty", name, k)
		case !slices.Equal(placeholders(v), placeholders(w)):
			t.Errorf("%s: %q has placeholders %v, English has %v", name, k, placeholders(w), placeholders(v))
		}
	}
	for k := range other {
		if _, ok := en[k]; !ok {
			t.Errorf("%s: %q is not in %s", name, k, enName)
		}
	}
}

// Persian text uses Persian letters and digits, and joins prefixes and suffixes with ZWNJ.
func TestPersianSpelling(t *testing.T) {
	files, _ := fs.Glob(localeFS, "locales/fa/*.json")
	for _, name := range files {
		for k, v := range readLocale(t, name) {
			if strings.ContainsAny(v, "يكى٠١٢٣٤٥٦٧٨٩") {
				t.Errorf("%s: %q has Arabic ي, ك, ى or digits; use ی, ک and ۰-۹", name, k)
			}
			for _, word := range strings.Fields(v) {
				switch strings.Trim(word, "،؛.:!؟…") {
				case "می", "نمی", "ها", "های", "هایی":
					t.Errorf("%s: %q has a detached %q; join it with ZWNJ (U+200C) as in می‌شود or حساب‌ها", name, k, word)
				}
			}
		}
	}
}

func TestDuplicateKeys(t *testing.T) {
	got := duplicateKeys([]byte(`{"a": "b", "b": "x", "a": "y"}`))
	if !slices.Equal(got, []string{"a"}) {
		t.Errorf("duplicateKeys = %q, want [a]", got)
	}
}

func TestLoadCatalogsMergesFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"locales/en/common.json": {Data: []byte(`{"a": "A"}`)},
		"locales/en/ops.json":    {Data: []byte(`{"b": "B"}`)},
		"locales/fa/common.json": {Data: []byte(`{"a": "الف"}`)},
	}
	cats := loadCatalogs(fsys)
	if got := cats["en"].t("b"); got != "B" {
		t.Errorf("en b = %q, want B", got)
	}
	if got := cats["fa"].t("b"); got != "B" {
		t.Errorf("fa b = %q, want the English fallback B", got)
	}
	if !cats["fa"].rules.rtl {
		t.Error("fa should be right to left")
	}

	fsys["locales/en/ops.json"] = &fstest.MapFile{Data: []byte(`{"a": "again"}`)}
	defer func() {
		if recover() == nil {
			t.Error("a key defined in two files should panic")
		}
	}()
	loadCatalogs(fsys)
}
