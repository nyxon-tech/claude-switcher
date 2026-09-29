package ops

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/nyxon-tech/claude-switcher/v3/internal/store"
)

// Save stores the login Desktop is signed into as profile name and makes it the profile in use.
// A profile of that name saved from another account is only replaced when replace is set;
// otherwise Save refuses with other-account, so the caller can ask first.
func (e *Env) Save(name string, replace bool) error {
	if !store.ValidName(name) {
		return errf(BadName, "name", name)
	}
	if !e.signedIn() {
		return errf(NotSignedIn)
	}
	if p, ok := e.Vault.Find(name); ok {
		name = p.Name
		if live := e.live(); !replace && p.Account != "" && live != "" && p.Account != live {
			return errf(OtherAccount, "name", name)
		}
	}
	if err := e.closedForLogin(); err != nil {
		return err
	}
	if err := e.saveLogin(name); err != nil {
		return err
	}
	return e.Vault.SetCurrent(name)
}

// Add gets Desktop ready to sign into another account without logging out, since logging out
// can end a saved login. It saves the profile in use, clears the login so Desktop opens signed
// out, and makes name the profile in use; the new login is saved at the next switch.
func (e *Env) Add(name string) error {
	if !store.ValidName(name) {
		return errf(BadName, "name", name)
	}
	if _, ok := e.Vault.Find(name); ok {
		return errf(ProfileExists, "name", name)
	}
	current, ok := e.Vault.Find(e.Vault.Current())
	if !ok {
		return errf(NoCurrent)
	}
	if err := e.matches(current); err != nil {
		return err
	}
	if err := e.closedForLogin(); err != nil {
		return err
	}
	if err := e.saveLogin(current.Name); err != nil {
		return err
	}
	if err := store.ClearLogin(e.Install.DataDir, e.Items); err != nil {
		return err
	}
	return e.Vault.SetCurrent(name)
}

// Switch saves the login in use under the current profile and restores profile name's login.
func (e *Env) Switch(name string) error {
	target, ok := e.Vault.Find(name)
	if !ok {
		return errf(NoProfile, "name", name)
	}
	current := e.Vault.Current()
	if current == target.Name {
		return errf(AlreadyCurrent, "name", current)
	}
	if p, ok := e.Vault.Find(current); ok {
		if err := e.matches(p); err != nil {
			return err
		}
	}
	if current == "" && e.unsavedLogin() {
		return errf(NoCurrent) // with no profile to save it under, the login would be lost
	}
	if err := e.closedForLogin(); err != nil {
		return err
	}
	if current != "" {
		if err := e.saveLogin(current); err != nil {
			return err
		}
	}
	if err := e.Vault.RestoreLogin(target.Name, e.Install.DataDir, e.Items); err != nil {
		return err
	}
	return e.Vault.SetCurrent(target.Name)
}

// Rename renames a saved login.
func (e *Env) Rename(old, name string) error {
	p, ok := e.Vault.Find(old)
	if !ok {
		return errf(NoProfile, "name", old)
	}
	if !store.ValidName(name) {
		return errf(BadName, "name", name)
	}
	if q, ok := e.Vault.Find(name); ok && q.Name != p.Name {
		return errf(ProfileExists, "name", name)
	}
	return e.Vault.Rename(p.Name, name)
}

// Remove forgets a saved login. Its chats stay in Desktop.
func (e *Env) Remove(name string) error {
	p, ok := e.Vault.Find(name)
	if !ok {
		return errf(NoProfile, "name", name)
	}
	if p.Name == e.Vault.Current() {
		return errf(ProfileInUse, "name", p.Name)
	}
	return e.Vault.Remove(p.Name)
}

// matches refuses when Desktop was signed into another account by hand: saving the profile in
// use would then overwrite its login with someone else's.
func (e *Env) matches(p store.Profile) error {
	if live := e.live(); p.Account != "" && live != "" && p.Account != live {
		return errf(OtherAccount, "name", p.Name)
	}
	return nil
}

// closedForLogin is closed plus stopping the helpers that keep login files open after Desktop
// quits.
func (e *Env) closedForLogin() error {
	if err := e.closed(); err != nil {
		return err
	}
	e.Desktop.StopHelpers()
	return nil
}

// saveLogin snapshots Desktop's login into profile name. Without config.json Desktop is signed
// out: there is nothing to save, and saving would wipe the profile's login.
func (e *Env) saveLogin(name string) error {
	if !e.signedIn() {
		return nil
	}
	if err := e.Vault.SaveLogin(name, e.Install.DataDir, e.Items); err != nil {
		return err
	}
	if live := e.live(); live != "" {
		return e.Vault.SetAccount(name, live)
	}
	return nil
}

func (e *Env) signedIn() bool {
	_, err := os.Stat(filepath.Join(e.Install.DataDir, "config.json"))
	return err == nil
}

// unsavedLogin reports whether Desktop holds a login that no profile has saved.
func (e *Env) unsavedLogin() bool {
	if !e.signedIn() {
		return false
	}
	live := e.live()
	ps, _ := e.Vault.Profiles()
	return live == "" || !slices.ContainsFunc(ps, func(p store.Profile) bool { return p.Account == live })
}
