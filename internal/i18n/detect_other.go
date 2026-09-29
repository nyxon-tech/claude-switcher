//go:build !windows

package i18n

// systemLanguages is empty: outside Windows the locale variables already carry the language.
func systemLanguages() []string { return nil }
