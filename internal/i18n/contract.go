// Package i18n holds every piece of text the app shows, in English and Persian, plus numbers,
// dates (Gregorian or Jalali) and relative times in the chosen language.
//
// Text lives in locales/<lang>/*.json: flat objects of dotted keys ("tab.chats") to strings
// with {name} placeholders. All files of a language are merged, so each part of the app keeps
// its own file (common.json, ops.json, ...). A key missing in a language falls back to English;
// a key missing in English shows as the key itself. Counted text has ".one" and ".other"
// variants ("count.chats.one"), picked by the integer argument "n".
//
// To add a language, drop locales/<code>/*.json with the same file names, keys and
// placeholders as locales/en (".one" variants may be left out when the language never uses
// them). If it is written right to left, has its own digits, uses the Jalali calendar or keeps
// nouns singular after numbers, add it to langRules. The catalog tests check the rest.
package i18n

import (
	"sync/atomic"
	"time"
)

var (
	current     atomic.Pointer[catalog]
	asciiDigits atomic.Bool // SetPersianDigits(false)
	gregorian   atomic.Bool // SetJalali(false)
)

func init() { current.Store(catalogs["en"]) }

// Load switches the language ("en" or "fa"; anything else means "en").
func Load(lang string) { current.Store(catalogs[supported(lang)]) }

// Lang is the current language code.
func Lang() string { return current.Load().lang }

// RTL reports whether the current language is written right to left.
func RTL() bool { return current.Load().rules.rtl }

// SetPersianDigits turns Persian digits (۰-۹) on or off for Persian. On by default.
func SetPersianDigits(on bool) { asciiDigits.Store(!on) }

// SetJalali turns the Jalali calendar on or off for Persian dates. On by default.
func SetJalali(on bool) { gregorian.Store(!on) }

// T returns the text for key in the current language. args are name, value pairs that fill
// {name} placeholders; an argument named "n" also picks the key's ".one" or ".other" variant
// when the catalog has them. Integers are written with N.
func T(key string, args ...any) string { return current.Load().t(key, args...) }

// N writes an integer with grouping in the current language (1,234 or ۱٬۲۳۴).
func N(v int64) string { return current.Load().n(v) }

// Compact writes a large count briefly: 1.2M, 34k (Persian: ۱٫۲ میلیون, ۳۴ هزار).
func Compact(v int64) string { return current.Load().compact(v) }

// Digits rewrites a number written in ASCII ("12.5%", "1,234.5") with the current language's
// digits, separators and percent sign ("۱۲٫۵٪"), for numbers N cannot write. Pass only the
// number: paths, IDs and versions should stay ASCII.
func Digits(s string) string { return current.Load().digits(s) }

// Ago is a relative time such as "5m ago" or "۵ دقیقه پیش"; older than 30 days gives Date(t).
func Ago(t, now time.Time) string { return current.Load().ago(t, now) }

// Date is t's calendar date in the current language (Jalali for Persian unless turned off).
// It uses t's own location, so pass t.Local() for the user's day.
func Date(t time.Time) string { return current.Load().date(t) }

// Detect guesses the user's language from the environment (CLAUDE_SWITCHER_LANG, LC_ALL,
// LC_MESSAGES, LANG, then the Windows display languages). It returns "fa" or "en".
func Detect() string { return detect() }
