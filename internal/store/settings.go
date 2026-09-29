package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Settings are the user's choices, kept in settings.json in the vault.
type Settings struct {
	Lang          string `json:"lang"`  // "en", "fa", or "" to follow the system
	Theme         string `json:"theme"` // "auto", "nyxon-dark", "nyxon-light", "contrast" or "plain"
	RTL           string `json:"rtl"`   // rtl mode: "auto", "app", "terminal" or "off"
	PersianDigits bool   `json:"persianDigits"`
	Jalali        bool   `json:"jalali"`
	UpdateCheck   bool   `json:"updateCheck"`
	Onboarded     bool   `json:"onboarded"`

	other map[string]json.RawMessage // keys a newer version wrote, kept as they are
}

// DefaultSettings are the settings before the user changes anything.
func DefaultSettings() Settings {
	return Settings{Theme: "auto", RTL: "auto", PersianDigits: true, Jalali: true, UpdateCheck: true}
}

// Settings reads settings.json; missing keys keep their defaults.
func (v Vault) Settings() (Settings, error) {
	s := DefaultSettings()
	data, err := os.ReadFile(v.settingsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s.other); err != nil {
		return DefaultSettings(), err
	}
	return s, json.Unmarshal(data, &s)
}

// SaveSettings writes settings.json, keeping keys this version does not know.
func (v Vault) SaveSettings(s Settings) error {
	known, err := json.Marshal(s)
	if err != nil {
		return err
	}
	all := map[string]json.RawMessage{}
	for k, raw := range s.other {
		all[k] = raw
	}
	if err := json.Unmarshal(known, &all); err != nil {
		return err
	}
	data, err := json.MarshalIndent(all, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(v.Dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(v.settingsPath(), data, 0o600)
}

func (v Vault) settingsPath() string { return filepath.Join(v.Dir, "settings.json") }
