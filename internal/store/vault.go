// Package store keeps Claude Switcher's own state in the vault (~/.claude-instances), in the same
// layout v2 used: one folder per saved login, _account, _current_profile, _journal, settings.json.
package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Vault is the folder that holds our state.
type Vault struct{ Dir string }

// Profile is one saved login.
type Profile struct {
	Name    string
	Account string    // account uuid the login belongs to, "" when unknown
	Dir     string    // the profile's folder
	Saved   time.Time // when the login was last saved
}

// Profiles lists every saved login: folders not starting with "_" that hold config.json.
func (v Vault) Profiles() ([]Profile, error) {
	entries, err := os.ReadDir(v.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		dir := filepath.Join(v.Dir, e.Name())
		st, err := os.Stat(filepath.Join(dir, "config.json"))
		if err != nil {
			continue
		}
		out = append(out, Profile{Name: e.Name(), Account: readLine(filepath.Join(dir, "_account")), Dir: dir, Saved: st.ModTime()})
	}
	return out, nil
}

// Find returns the profile called name. Names match without regard to case, as they do in the
// Windows file system.
func (v Vault) Find(name string) (Profile, bool) {
	ps, _ := v.Profiles()
	for _, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Profile{}, false
}

// Current is the profile Desktop is meant to be signed into, "" before the first save.
func (v Vault) Current() string { return readLine(filepath.Join(v.Dir, "_current_profile")) }

// SetCurrent records the profile in use.
func (v Vault) SetCurrent(name string) error {
	return v.writeFile(filepath.Join(v.Dir, "_current_profile"), name)
}

// SetAccount records which account a profile's login belongs to.
func (v Vault) SetAccount(name, account string) error {
	return v.writeFile(filepath.Join(v.Dir, name, "_account"), account)
}

// Rename renames a profile, and the current profile with it.
func (v Vault) Rename(old, name string) error {
	if err := os.Rename(filepath.Join(v.Dir, old), filepath.Join(v.Dir, name)); err != nil {
		return err
	}
	if v.Current() == old {
		return v.SetCurrent(name)
	}
	return nil
}

// Remove deletes a saved login.
func (v Vault) Remove(name string) error { return os.RemoveAll(filepath.Join(v.Dir, name)) }

func (v Vault) writeFile(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

func readLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// nameRule is v2's rule, widened to combining marks and ZWNJ inside a name (Persian needs it).
var nameRule = regexp.MustCompile(`^[\p{L}\p{N}](?:[\p{L}\p{M}\p{N} ._\x{200C}-]{0,38}[\p{L}\p{M}\p{N}_-])?$`)

// ValidName reports whether name can be a profile: up to 40 letters, digits, spaces, dots,
// dashes or underscores, starting with a letter or digit, and not a name Windows reserves.
func ValidName(name string) bool {
	if !nameRule.MatchString(name) {
		return false
	}
	base, _, _ := strings.Cut(name, ".")
	switch base = strings.ToUpper(strings.TrimSpace(base)); base {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	return !(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9')
}
