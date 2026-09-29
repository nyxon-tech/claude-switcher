package i18n

import "os"

// detect follows POSIX: the first locale variable that is set decides. Only when none is set
// does the OS setting count.
func detect() string {
	for _, name := range []string{"CLAUDE_SWITCHER_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); v != "" {
			return supported(v)
		}
	}
	for _, locale := range systemLanguages() {
		if lang := baseLang(locale); catalogs[lang] != nil {
			return lang
		}
	}
	return "en"
}
