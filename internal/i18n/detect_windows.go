package i18n

import "golang.org/x/sys/windows"

// systemLanguages is the user's Windows display languages, most preferred first ("fa-IR").
func systemLanguages() []string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return langs
}
