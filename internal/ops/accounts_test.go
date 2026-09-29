package ops

import (
	"os"
	"path/filepath"
	"testing"
)

// The v2 "profiles" fixture: save, add, switch both ways, refusals, names.
func TestAccounts(t *testing.T) {
	f := newFixture(t)
	f.signIn(work, "work")
	f.put(filepath.Join(f.data, "Local State"), "shared-dpapi-key")
	f.put(filepath.Join(f.data, "claude_desktop_config.json"), `{"mcpServers":{}}`)
	shared := func() string {
		return f.read(filepath.Join(f.data, "Local State")) + f.read(filepath.Join(f.data, "claude_desktop_config.json"))
	}
	sharedBefore, workLogin := shared(), f.login()

	must(t, f.Save("work", false))
	if f.read(filepath.Join(f.Vault.Dir, "work", "_account")) != work || f.Vault.Current() != "work" {
		t.Fatal("save should record the account and mark the profile in use")
	}

	must(t, f.Add("personal"))
	if f.exists(filepath.Join(f.data, "config.json")) || f.exists(filepath.Join(f.data, "Network")) {
		t.Error("add should clear the login so Desktop opens signed out")
	}
	if shared() != sharedBefore {
		t.Error("add touched files outside the login")
	}
	st, err := f.Status()
	must(t, err)
	if st.Current != "personal" || len(st.Profiles) != 1 || st.Account != "" {
		t.Errorf("status after add: current %q, %d profiles, account %q", st.Current, len(st.Profiles), st.Account)
	}

	f.signIn(pers, "personal")
	must(t, os.Remove(filepath.Join(f.data, "DIPS-wal"))) // personal has none
	personalLogin := f.login()

	must(t, f.Switch("work"))
	if f.read(filepath.Join(f.Vault.Dir, "personal", "_account")) != pers {
		t.Error("switch should save the account it leaves")
	}
	if f.login() != workLogin {
		t.Error("switch should restore the work login byte for byte")
	}
	must(t, f.Switch("personal"))
	if f.login() != personalLogin {
		t.Error("switching back should restore personal byte for byte")
	}
	if f.exists(filepath.Join(f.data, "DIPS-wal")) {
		t.Error("a stale DIPS-wal from the other account survived")
	}

	// Desktop signed into a third account by hand: nothing may change.
	f.put(filepath.Join(f.data, "config.json"), `{"lastKnownAccountUuid":"`+third+`"}`)
	tampered, saved := f.login(), f.hash(filepath.Join(f.Vault.Dir, "personal"))
	wantCode(t, f.Switch("work"), OtherAccount)
	wantCode(t, f.Add("other"), OtherAccount)
	wantCode(t, f.Save("personal", false), OtherAccount)
	if f.login() != tampered || f.hash(filepath.Join(f.Vault.Dir, "personal")) != saved {
		t.Error("a refused switch changed something")
	}
	f.put(filepath.Join(f.data, "config.json"), `{"lastKnownAccountUuid":"`+pers+`"}`)

	if f.desk.launches != 0 || f.desk.helpers == 0 {
		t.Errorf("launches %d, helpers stopped %d: ops never launches, and stops helpers before saving", f.desk.launches, f.desk.helpers)
	}
	if shared() != sharedBefore {
		t.Error("files outside the login changed")
	}
}

func TestAccountRefusals(t *testing.T) {
	f := newFixture(t)
	wantCode(t, f.Save("work", false), NotSignedIn)
	f.signIn(work, "work")
	wantCode(t, f.Add("personal"), NoCurrent)
	for _, name := range []string{"_hidden", "trailing.", "", "a/b", "CON"} {
		wantCode(t, f.Save(name, false), BadName)
	}
	if ps, _ := f.Vault.Profiles(); len(ps) != 0 {
		t.Fatal("a refused name was saved")
	}
	must(t, f.Save("work", false))
	wantCode(t, f.Add("Work"), ProfileExists)
	wantCode(t, f.Switch("nope"), NoProfile)
	wantCode(t, f.Switch("work"), AlreadyCurrent)
	wantCode(t, f.Remove("work"), ProfileInUse)
	wantCode(t, f.Rename("nope", "x"), NoProfile)
	wantCode(t, f.Rename("work", "bad/name"), BadName)

	// saving over a profile of another account needs replace
	f.signIn(pers, "personal")
	wantCode(t, f.Save("work", false), OtherAccount)
	must(t, f.Save("work", true))
	if p, _ := f.Vault.Find("work"); p.Account != pers {
		t.Error("replace should save the signed-in account")
	}
}

func TestRenameAndRemove(t *testing.T) {
	f := newFixture(t)
	f.signIn(work, "work")
	must(t, f.Save("work", false))
	must(t, f.Add("personal"))
	f.signIn(pers, "personal")
	must(t, f.Switch("work"))

	persian := "کار"
	must(t, f.Rename("work", persian))
	if _, ok := f.Vault.Find(persian); !ok || f.Vault.Current() != persian {
		t.Fatal("rename should keep the profile in use, Persian names included")
	}
	wantCode(t, f.Rename(persian, "personal"), ProfileExists)
	must(t, f.Rename(persian, "work"))
	must(t, f.Remove("personal"))
	if _, ok := f.Vault.Find("personal"); ok {
		t.Error("remove left the profile")
	}
}

// With no profile in use, a switch must not throw away a login that no profile holds.
func TestSwitchKeepsUnsavedLogin(t *testing.T) {
	f := newFixture(t)
	f.signIn(pers, "personal")
	must(t, f.Save("personal", false))
	noCurrent := func() { must(t, os.Remove(filepath.Join(f.Vault.Dir, "_current_profile"))) }
	noCurrent()
	f.signIn(work, "work")
	before := f.login()
	wantCode(t, f.Switch("personal"), NoCurrent)
	if f.login() != before {
		t.Fatal("a refused switch changed the login")
	}
	must(t, f.Save("work", false))
	noCurrent()
	must(t, f.Switch("personal")) // the work login is saved now, so nothing is lost
}

// A Desktop without config.json (signed out, or reinstalled) must never be saved over a profile.
func TestSwitchKeepsSavedLoginWhenSignedOut(t *testing.T) {
	f := newFixture(t)
	f.signIn(pers, "personal")
	must(t, f.Save("personal", false))
	f.signIn(work, "work")
	must(t, f.Save("work", false))
	saved := f.hash(filepath.Join(f.Vault.Dir, "work"))

	must(t, os.Remove(filepath.Join(f.data, "config.json")))
	must(t, f.Switch("personal"))
	if f.hash(filepath.Join(f.Vault.Dir, "work")) != saved {
		t.Fatal("the work login was overwritten by a signed-out Desktop")
	}
	if f.Vault.Current() != "personal" || f.read(filepath.Join(f.data, "config.json")) == "" {
		t.Error("switch should still restore personal")
	}
}
