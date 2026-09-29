// Package i18n holds every piece of text the app shows, and writes numbers, dates and relative
// times. The app is in English; its catalogs are where another language would be added.
//
// Text lives in locales/en/*.json: flat objects of dotted keys ("tab.chats") to strings with
// {name} placeholders. The files are merged when the package loads, so each part of the app
// keeps its own file (common.json, ops.json, ...). A key missing from the catalog shows as the
// key itself. Counted text has ".one" and ".other" variants ("count.chats.one"), picked by the
// integer argument "n". The catalog tests check that every key the code names exists.
package i18n

// T returns the text for key. args are name, value pairs that fill {name} placeholders; an
// argument named "n" also picks the key's ".one" or ".other" variant when the catalog has them.
// Integers are written with N.
func T(key string, args ...any) string { return english.t(key, args...) }
